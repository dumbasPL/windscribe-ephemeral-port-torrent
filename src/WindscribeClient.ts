import AsyncLock from 'async-lock';
import {AxiosResponse, default as axios} from 'axios';
import {default as Keyv, KeyvStoreAdapter} from 'keyv';
import {Cookie, parse as parseCookie} from 'set-cookie-parser';
import qs from 'qs';


const lock = new AsyncLock();

const userAgent = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36';

interface CsrfInfo {
  csrfTime: number;
  csrfToken: string;
}

interface PortForwardingInfo {
  epfExpires: number;
  ports: number[];
}

export interface WindscribePort {
  port: number,
  expires: Date,
}

export class WindscribeClient {

  private cache: Keyv<string>;

  constructor(
    private username: string,
    private password: string,
    cache?: KeyvStoreAdapter,
  ) {
    this.cache = new Keyv({
      store: cache,
      namespace: 'windscribe',
    });
  }

  async updatePort(): Promise<WindscribePort> {
    // get csrf token and time to pass on to future requests
    // this will also verify if we are logged in and login if not
    const csrfToken = await this.getMyAccountCsrfToken();

    // check for current status
    let portForwardingInfo = await this.getPortForwardingInfo();

    // check for mismatched ports if any present
    if (portForwardingInfo.ports.length == 2 && portForwardingInfo.ports[0] != portForwardingInfo.ports[1]) {
      console.log('Detected mismatched ports, removing existing ports');
      await this.removeEphemeralPort(csrfToken);

      // update data to match current state
      portForwardingInfo.ports = [];
      portForwardingInfo.epfExpires = 0;
      await this.cache.delete('port');
    }

    // request new port if we don't have any
    if (portForwardingInfo.epfExpires == 0) {
      console.log('No windscribe port configured, requesting new matching ephemeral port');
      portForwardingInfo = await this.requestMatchingEphemeralPort(csrfToken);
    } else {
      console.log(`Using existing windscribe ephemeral port: ${portForwardingInfo.ports[0]}`);
    }

    const ret = {
      port: portForwardingInfo.ports[0],
      expires: new Date((portForwardingInfo.epfExpires + 86400 * 7) * 1000),
    };

    await this.cache.set('port', ret.port.toString(), ret.expires.getTime() - Date.now());

    return ret;
  }

  async getPort(): Promise<WindscribePort | null> {
    const cachedPort = await this.cache.get('port', {raw: true});
    return !cachedPort?.value || !cachedPort?.expires ? null : {
      port: parseInt(cachedPort.value),
      expires: new Date(cachedPort.expires),
    };
  }

  private async getSession(forceLogin: boolean = false): Promise<string> {
    return lock.acquire('getSession', async () => {
      if (forceLogin) {
        // force clear the session
        await this.cache.delete('sessionCookie');
      } else {
        // try to get cached value
        const cachedCookie = await this.cache.get('sessionCookie');
        if (cachedCookie != undefined) {
          return cachedCookie;
        }
      }

      // get a new session
      console.log(`Invalid/missing session cookie, logging into windscribe`);
      const sessionCookie = await this.login();
      if (!sessionCookie.expires) {
        throw new Error('Windscribe session cookie does not have an expiration date');
      }
      await this.cache.set('sessionCookie', sessionCookie.value, sessionCookie.expires.getTime() - Date.now());
      console.log(`Successfully logged into windscribe, session expires in ${Math.floor((sessionCookie.expires.getTime() - Date.now()) / (100 * 60)) / 10} minutes`);

      return sessionCookie.value;
    });
  }

  private async login(): Promise<Cookie> {
    try {
      // get csrf token and time
      const {data: csrfData} = await axios.post<{csrf_token: string, csrf_time: number}>('https://res.windscribe.com/res/logintoken', null, {
        headers: {'User-Agent': userAgent},
      });

      // log in
      const res = await axios.post('https://windscribe.com/login', qs.stringify({
        login: '1',
        upgrade: '0',
        csrf_time: csrfData.csrf_time,
        csrf_token: csrfData.csrf_token,
        username: this.username,
        password: this.password,
        code: ''
      }), {
        headers: {'content-type': 'application/x-www-form-urlencoded', 'User-Agent': userAgent},
        maxRedirects: 0,
        validateStatus: status => status == 302,
      });
      const setCookieHeader = res.headers['set-cookie'];
      if (!setCookieHeader) {
        throw new Error('No set-cookie header found in login response');
      }

      // extract the cookie
      return parseCookie(setCookieHeader, {map: true, decodeValues: true})['ws_session_auth_hash'];
    } catch (error) {
      // try to extract windscribe message
      if (axios.isAxiosError(error) && error.response) {
        const response = error.response as AxiosResponse<string>;
        const errorMessage = /<div class="content_message error">.*>(.*)<\/div/.exec(response.data);
        if (response.status == 200 && errorMessage && errorMessage[1]) {
          throw new Error(`Failed to log into windscribe: ${errorMessage[1]}`);
        }
      }

      // or throw a generic error if windscribe message not found
      throw new Error(`Failed to log into windscribe: ${error instanceof Error ? error.message : String(error)}`);
    }
  }

  private async getMyAccountCsrfToken(forceLogin: boolean = false): Promise<CsrfInfo> {
    try {
      const sessionCookie = await this.getSession(forceLogin);

      // get page
      const res = await axios.get<string>('https://windscribe.com/myaccount', {
        headers: {
          'Cookie': `ws_session_auth_hash=${sessionCookie};`,
          'User-Agent': userAgent,
        },
        maxRedirects: 0,
        validateStatus: status => [302, 200].includes(status),
      });

      if (res.status == 302) {
        // force to login again as the current session is invalid
        return await this.getMyAccountCsrfToken(true);
      }

      // extract csrf tokena and time from page content
      const csrfTime = /csrf_time = (\d+);/.exec(res.data);
      const csrfToken = /csrf_token = '(\w+)';/.exec(res.data);
      if (!csrfTime || !csrfToken) {
        throw new Error('Failed to extract csrf token and time from my account page');
      }

      return {
        csrfTime: +csrfTime[1],
        csrfToken: csrfToken[1],
      };
    } catch (error) {
      throw new Error(`Failed to get csrf token from my account page: ${error instanceof Error ? error.message : String(error)}`);
    }
  }

  private async getPortForwardingInfo(): Promise<PortForwardingInfo> {
    try {
      const sessionCookie = await this.getSession();

      // load sub page
      const res = await axios.get<string>('https://windscribe.com/staticips/load', {
        headers: {
          'Cookie': `ws_session_auth_hash=${sessionCookie};`,
          'User-Agent': userAgent,
        }
      });

      // extract data from page
      const epfExpires = res.data.match(/epfExpires = (\d+);/); // this is always present. set to 0 if no port is active
      const ports = [...res.data.matchAll(/<span>(?<port>\d+)<\/span>/g)].map(x => +x[1]); // this will return an empty array when there are not pots forwarded
      if (!epfExpires) {
        throw new Error('Failed to extract epfExpires from static IPs page');
      }

      return {
        epfExpires: +epfExpires[1],
        ports,
      };
    } catch (error) {
      throw new Error(`Failed to get port forwarding info: ${error instanceof Error ? error.message : String(error)}`);
    }
  }

  private async removeEphemeralPort(csrfInfo: CsrfInfo): Promise<void> {
    try {
      const sessionCookie = await this.getSession();

      // remove port
      const res = await axios.post<{success: number, epf: boolean, message?: string}>('https://windscribe.com/staticips/deleteEphPort', qs.stringify({
        ctime: csrfInfo.csrfTime,
        ctoken: csrfInfo.csrfToken
      }), {
        headers: {
          'content-type': 'application/x-www-form-urlencoded',
          'Cookie': `ws_session_auth_hash=${sessionCookie};`,
          'User-Agent': userAgent,
        }
      });

      // check for errors
      if (res.data.success == 0) {
        throw new Error(`success = 0; ${res.data.message ?? 'No message'}`);
      }

      // make sure we actually removed it
      if (res.data.epf == false) {
        console.warn('Tried to remove a non-existent ephemeral port, ignoring');
      } else {
        console.log('Deleted ephemeral port');
      }
    } catch (error) {
      throw new Error(`Failed to delete ephemeral port: ${error instanceof Error ? error.message : String(error)}`);
    }
  }

  private async requestMatchingEphemeralPort(csrfInfo: CsrfInfo): Promise<PortForwardingInfo> {
    try {
      const sessionCookie = await this.getSession();

      // request new port
      const res = await axios.post<{success: number, message?: string, epf?: {ext: number, int: number, start_ts: number}}>('https://windscribe.com/staticips/postEphPort', qs.stringify({
        ctime: csrfInfo.csrfTime,
        ctoken: csrfInfo.csrfToken,
        port: '', // empty string for a matching port
      }), {
        headers: {
          'content-type': 'application/x-www-form-urlencoded',
          'Cookie': `ws_session_auth_hash=${sessionCookie};`,
          'User-Agent': userAgent,
        }
      });

      // check for errors
      if (res.data.success == 0) {
        throw new Error(`success = 0; ${res.data.message ?? 'No message'}`);
      }

      // epf should be present by this point
      const epf = res.data.epf!;
      console.log(`Created new matching ephemeral port: ${epf.ext}`);
      return {
        epfExpires: epf.start_ts,
        ports: [epf.ext, epf.int],
      };
    } catch (error) {
      throw new Error(`Failed to request matching ephemeral port: ${error instanceof Error ? error.message : String(error)}`);
    }
  }

}
