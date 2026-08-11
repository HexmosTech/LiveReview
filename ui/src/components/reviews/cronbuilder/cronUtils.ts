import cronstrue from 'cronstrue';

export interface CronTextResult {
  status: boolean;
  value?: string;
}

// Ported from https://github.com/vpfaiz/cron-builder-ui (MIT).
export function getCronText(cronString: string): CronTextResult {
  try {
    const value = cronstrue.toString(cronString.trim());
    return { status: true, value };
  } catch (error) {
    return { status: false };
  }
}
