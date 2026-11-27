export const SITE_URL = 'https://tarkov.nexus';

export const DISCORD_URL = 'https://discord.gg/RfsVFVCS5j';

export interface Author {
  name: string;
  login: string;
  role: string;
}

export const AUTHORS: Author[] = [
  { name: 'uBlue', login: 'ImStillBlue', role: 'Creator' },
  { name: 'Jordan', login: 'Jordanmuss99', role: 'Co-developer' }
];

export const githubProfile = (login: string) => `https://github.com/${login}`;
