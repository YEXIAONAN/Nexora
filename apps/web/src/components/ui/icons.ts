export type IconName =
  | 'overview'
  | 'activity'
  | 'sun'
  | 'moon'
  | 'monitor'
  | 'refresh'
  | 'menu'
  | 'close'
  | 'database'
  | 'server'
  | 'shield'
  | 'arrow-up-right'
  | 'users'
  | 'inbox'
  | 'logout'
  | 'plus'

export const iconPaths: Record<IconName, string[]> = {
  overview: ['M4 4h6v6H4z', 'M14 4h6v6h-6z', 'M4 14h6v6H4z', 'M14 14h6v6h-6z'],
  activity: ['M3 12h4l2.5-6 4 12L16 12h5'],
  sun: [
    'M12 4V2',
    'M12 22v-2',
    'M4 12H2',
    'M22 12h-2',
    'M5.6 5.6 4.2 4.2',
    'M19.8 19.8l-1.4-1.4',
    'M18.4 5.6l1.4-1.4',
    'M4.2 19.8l1.4-1.4',
    'M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0z',
  ],
  moon: ['M21 12.8A8.5 8.5 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z'],
  monitor: ['M3 4h18v12H3z', 'M8 20h8', 'M12 16v4'],
  refresh: ['M20 11a8 8 0 1 0-.6 4', 'M20 4v7h-7'],
  menu: ['M4 6h16', 'M4 12h16', 'M4 18h16'],
  close: ['M6 6l12 12', 'M18 6 6 18'],
  database: [
    'M12 3c4.4 0 8 1.3 8 3s-3.6 3-8 3-8-1.3-8-3 3.6-3 8-3z',
    'M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6',
    'M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3',
  ],
  server: ['M4 4h16v6H4z', 'M4 14h16v6H4z', 'M8 7h.01', 'M8 17h.01'],
  shield: ['M12 3 5 6v5c0 4.4 3 8.4 7 10 4-1.6 7-5.6 7-10V6l-7-3z', 'M9 12l2 2 4-4'],
  'arrow-up-right': ['M7 17 17 7', 'M8 7h9v9'],
  users: [
    'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2',
    'M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8z',
    'M22 21v-2a4 4 0 0 0-3-3.87',
    'M16 3.13a4 4 0 0 1 0 7.75',
  ],
  inbox: [
    'M22 12h-6l-2 3h-4l-2-3H2',
    'M5.4 5.1 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.4-6.9A2 2 0 0 0 16.8 4H7.2a2 2 0 0 0-1.8 1.1z',
  ],
  logout: ['M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4', 'M16 17l5-5-5-5', 'M21 12H9'],
  plus: ['M12 5v14', 'M5 12h14'],
}
