export type ConnectionsData = {
  url: string;
  serviceUsername?: string;
  configured: boolean;
  personalUsername: string;
  deezerConfigured: boolean;
  losslessConfigured?: boolean;
  losslessEmail?: string;
  servicePasswordConfigured?: boolean;
  personalPasswordConfigured?: boolean;
  downloaderReady: boolean;
  musicMounted: boolean;
  downloaderVersion: string;
};
export type ConnectionDraft = {
  url: string;
  username: string;
  password: string;
  arl: string;
};

export const emptyConnection: ConnectionDraft = {
  url: "",
  username: "",
  password: "",
  arl: "",
};
