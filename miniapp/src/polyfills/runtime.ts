const runtimeGlobal =
  typeof globalThis !== 'undefined'
    ? globalThis
    : typeof global !== 'undefined'
      ? global
      : typeof self !== 'undefined'
        ? self
        : typeof window !== 'undefined'
          ? window
          : typeof tt !== 'undefined'
            ? tt
            : typeof wx !== 'undefined'
              ? wx
              : {}

if (typeof runtimeGlobal.globalThis === 'undefined') {
  runtimeGlobal.globalThis = runtimeGlobal
}

export {}
