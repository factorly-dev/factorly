// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package ui

import "embed"

//go:embed static/*
var staticFS embed.FS

//go:embed templates/*.html templates/partials/*.html
var templatesFS embed.FS
