// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

//go:generate gotext extract --lang=de,zh

import (
	"net/http"

	"github.com/yyleeshine/mpquic/repository/x/text/cmd/gotext/examples/extract_http/pkg"
)

func main() {
	http.Handle("/generize", http.HandlerFunc(pkg.Generize))
}
