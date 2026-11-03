export const route = $state({ hash: location.hash })

window.addEventListener('hashchange', () => (route.hash = location.hash))

// '#/opnames/12' → routeParam() = '12'; '#/settlements/new/3' → routeParam(1) = '3'
export const routeParam = (index = 0) => route.hash.split('/')[2 + index] ?? ''
