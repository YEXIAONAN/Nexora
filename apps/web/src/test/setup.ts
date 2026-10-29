const mediaQueryList = (query: string): MediaQueryList =>
  ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    addListener: () => undefined,
    removeListener: () => undefined,
    dispatchEvent: () => false,
  }) as unknown as MediaQueryList

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  configurable: true,
  value: mediaQueryList,
})
