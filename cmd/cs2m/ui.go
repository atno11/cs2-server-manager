package main

import _ "embed"

//go:embed ui.html
var htmlPage string

//go:embed ui.js
var browserScript string
