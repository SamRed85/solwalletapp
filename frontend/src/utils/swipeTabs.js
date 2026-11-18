let bound = null

export function bindSwipeTabs (actions) {
  bound = actions
  return () => {
    if (bound === actions) bound = null
  }
}

export function swipeTabs () {
  return bound
}
