type IconName = 'search' | 'filter' | 'chevron' | 'back' | 'upload' | 'download' | 'copy' | 'mail' | 'lock' | 'check'

const paths: Record<IconName, string> = {
  search: 'm21 21-5.2-5.2M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0Z',
  filter: 'M3 5h18M6 12h12m-8 7h4',
  chevron: 'm9 5 7 7-7 7',
  back: 'm14 5-7 7 7 7',
  upload: 'M12 16V3m-5 5 5-5 5 5M3 15v6h18v-6',
  download: 'M12 3v13m-5-5 5 5 5-5M3 15v6h18v-6',
  copy: 'M8 8h13v13H8Zm8 0V3H3v13h5',
  mail: 'M3 5h18v14H3Zm0 0 9 8 9-8',
  lock: 'M5 10h14v11H5Zm3 0V6a4 4 0 0 1 8 0v4',
  check: 'm4 12 5 5L20 6',
}

export function Icon({ name, className = '' }: { name: IconName; className?: string }) {
  return <svg className={`icon ${className}`} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d={paths[name]} /></svg>
}
