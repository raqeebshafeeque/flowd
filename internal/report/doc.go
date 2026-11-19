// Package report draws reviews as rows of text: a day with its stats and a
// timeline, a week with a bar chart of focused hours, and a month as a
// calendar heatmap. The command line and the workspace both draw with it,
// so a review looks the same in either.
//
// Every row is laid out on plain text first and colored last, so color
// never changes alignment, and nothing is wider than the width it is given.
package report
