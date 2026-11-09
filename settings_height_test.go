package main

// settingsWindowHeight is the shared window floor after the sixth module page
// gained its selection row (five 48/64 pt rows + two cards = 444 content,
// plus 48 padding). Every headless render and every minimum-size assertion
// uses it instead of a scattered literal.
const settingsWindowHeight = 492
