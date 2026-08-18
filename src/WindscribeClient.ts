const userAgent = 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36';

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

  private portCache: WindscribePort | null = null;

  constructor(private authHash: string) {}

  private async request<T>(method: string, path: string, body?: string, headers?: Record<string, string>): Promise<T> {
    const response = await fetch(`https://windscribe.com${path}`, {
      method,
      headers: {
        'Cookie': `ws_session_auth_hash=${this.authHash};`,
        'User-Agent': userAgent,
        ...headers,
      },
      body,
      redirect: 'manual',
    });

    if (!response.ok) {
      throw new Error(`Failed to fetch ${path}: ${response.status} ${response.statusText}`);
    }

    // windscribe will sometimes return a JSON response with text/html content type
    const res = await response.text();
    try {
      return JSON.parse(res) as T;
    } catch (error) {
      return res as unknown as T;
    }
  }

  private async get<T>(path: string): Promise<T> {
    return this.request<T>('GET', path);
  }

  private async post<T>(path: string, body: Record<string, any>): Promise<T> {
    const formBody = new URLSearchParams();
    for (const [key, value] of Object.entries(body)) {
      formBody.append(key, String(value));
    }
    return this.request<T>('POST', path, formBody.toString(), {
      'Content-Type': 'application/x-www-form-urlencoded',
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
      this.portCache = null;
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

    this.portCache = ret;
    return ret;
  }

  async getPort(): Promise<WindscribePort | null> {
    return this.portCache;
  }

  private async getMyAccountCsrfToken(): Promise<CsrfInfo> {
    try {
      // get page
      const res = await this.get<string>('/myaccount');

      // extract csrf tokena and time from page content
      const csrfTime = /csrf_time = (\d+);/.exec(res);
      const csrfToken = /csrf_token = '(\w+)';/.exec(res);
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
      // load sub page
      const res = await this.get<string>('/staticips/load');

      // extract data from page
      const epfExpires = res.match(/epfExpires = (\d+);/); // this is always present. set to 0 if no port is active
      const pfExt = res.match(/<span class="pf-ext">(\d+)<\/span>/); // this is only present if a port is active
      const pfInt = res.match(/<span class="pf-int">(\d+)<\/span>/); // this is only present if a port is active
      if (!epfExpires) {
        throw new Error('Failed to extract epfExpires from static IPs page');
      }

      return {
        epfExpires: +epfExpires[1],
        ports: pfExt && pfInt ? [+pfExt[1], +pfInt[1]] : [],
      };
    } catch (error) {
      throw new Error(`Failed to get port forwarding info: ${error instanceof Error ? error.message : String(error)}`);
    }
  }

  private async removeEphemeralPort(csrfInfo: CsrfInfo): Promise<void> {
    try {
      // remove port
      const res = await this.post<{
        success: number,
        epf: boolean,
        message?: string
      }>(
        '/staticips/deleteEphPort',
        {ctime: csrfInfo.csrfTime, ctoken: csrfInfo.csrfToken}
      );

      // check for errors
      if (res.success == 0) {
        throw new Error(`success = 0; ${res.message ?? 'No message'}`);
      }

      // make sure we actually removed it
      if (res.epf == false) {
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
      // request new port
      const res = await this.post<{
        success: number,
        message?: string,
        epf?: {
          ext: number,
          int: number,
          start_ts: number
        }
      }>(
        '/staticips/postEphPort',
        {ctime: csrfInfo.csrfTime, ctoken: csrfInfo.csrfToken}
      );

      // check for errors
      if (res.success == 0) {
        throw new Error(`success = 0; ${res.message ?? 'No message'}`);
      }

      // epf should be present by this point
      const epf = res.epf!;
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
