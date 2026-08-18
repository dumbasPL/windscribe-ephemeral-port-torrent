
interface ConfigTemplate<T extends string | number> {
  envVariableName: string,
  type: T extends string ? typeof String : typeof Number,
}

interface ConfigTemplateRequiredEntry<T extends string | number> extends ConfigTemplate<T> {
  required: true,
}

interface ConfigTemplateOptionalEntry<T extends string | number> extends ConfigTemplate<T> {
  required: false,
  default?: string,
}

const configTemplate = {
  delugeUrl: {
    envVariableName: 'DELUGE_URL',
    required: true,
    type: String,
  } as ConfigTemplateRequiredEntry<string>,
  delugePassword: {
    envVariableName: 'DELUGE_PASSWORD',
    required: true,
    type: String,
  } as ConfigTemplateRequiredEntry<string>,
  delugeHostId: {
    envVariableName: 'DELUGE_HOST_ID',
    required: false,
    type: String,
  } as ConfigTemplateOptionalEntry<string>,
  delugeRetryDelay: {
    envVariableName: 'DELUGE_RETRY_DELAY',
    required: false,
    default: `${5 * 60 * 1000}`, // 5 minutes
    type: Number,
  } as ConfigTemplateOptionalEntry<number>,
  windscribeAuthHash: {
    envVariableName: 'WINDSCRIBE_AUTH_HASH',
    required: true,
    type: String,
  } as ConfigTemplateRequiredEntry<string>,
  windscribeRetryDelay: {
    envVariableName: 'WINDSCRIBE_RETRY_DELAY',
    required: false,
    default: `${60 * 60 * 1000}`, // one hour
    type: Number,
  } as ConfigTemplateOptionalEntry<number>,
  windscribeExtraDelay: {
    envVariableName: 'WINDSCRIBE_EXTRA_DELAY',
    required: false,
    default: `${60 * 1000}`, // one minute
    type: Number,
  } as ConfigTemplateOptionalEntry<number>,
  cronSchedule: {
    envVariableName: 'CRON_SCHEDULE',
    required: false,
    type: String,
  } as ConfigTemplateOptionalEntry<string>,
};

type entryType =
  ConfigTemplateRequiredEntry<string> |
  ConfigTemplateOptionalEntry<string> |
  ConfigTemplateRequiredEntry<number> |
  ConfigTemplateOptionalEntry<number>;

type configTemplateType = typeof configTemplate;

type Config =
  {[key in keyof configTemplateType as configTemplateType[key] extends ConfigTemplateRequiredEntry<string> ? key : never]: string} &
  {[key in keyof configTemplateType as configTemplateType[key] extends ConfigTemplateOptionalEntry<string> ? key : never]?: string} &
  {[key in keyof configTemplateType as configTemplateType[key] extends ConfigTemplateOptionalEntry<number> ? key : never]?: number} &
  {[key in keyof configTemplateType as configTemplateType[key] extends ConfigTemplateOptionalEntry<number> ? key : never]?: number};

export function getConfig(): Config {
  const entries = Object.entries(configTemplate).map(([name, entry]: [string, entryType]) => {
    let value = process.env[entry.envVariableName];

    // this needs an explicit `== true` check because typescript
    if (entry.required == true) {
      if (!value || value.length == 0) {
        throw new Error(`Missing environment variable ${entry.envVariableName}`);
      }
    } else {
      value = value || entry.default || '';
    }

    if (entry.type == Number) {
      const intValue = parseInt(value);
      if (isNaN(intValue)) {
        throw new Error(`Environment variable ${entry.envVariableName}`);
      }
      return [name, intValue];
    }

    return [name, value ? value : null];
  });

  return Object.fromEntries(entries);
}
