// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/constant"
	"go/format"
	"go/token"
	"go/types"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yyleeshine/mpquic/repository/x/text/internal"
	fmtparser "github.com/yyleeshine/mpquic/repository/x/text/internal/format"
	"github.com/yyleeshine/mpquic/repository/x/text/language"
	"golang.org/x/tools/go/loader"
)

// TODO:
// - merge information into existing files
// - handle different file formats (PO, XLIFF)
// - handle features (gender, plural)
// - message rewriting

var (
	srcLang *string
	lang    *string
)

func init() {
	srcLang = cmdExtract.Flag.String("srclang", "en-US", "the source-code language")
	lang = cmdExtract.Flag.String("lang", "en-US", "comma-separated list of languages to process")
}

var cmdExtract = &Command{
	Run:       runExtract,
	UsageLine: "extract <package>*",
	Short:     "extract strings to be translated from code",
}

func runExtract(cmd *Command, args []string) error {
	conf := loader.Config{}
	prog, err := loadPackages(&conf, args)
	if err != nil {
		return wrap(err, "")
	}

	// print returns Go syntax for the specified node.
	print := func(n ast.Node) string {
		var buf bytes.Buffer
		format.Node(&buf, conf.Fset, n)
		return buf.String()
	}

	var messages []Message

	for _, info := range prog.AllPackages {
		for _, f := range info.Files {
			// Associate comments with nodes.
			cmap := ast.NewCommentMap(prog.Fset, f, f.Comments)
			getComment := func(n ast.Node) string {
				cs := cmap.Filter(n).Comments()
				if len(cs) > 0 {
					return strings.TrimSpace(cs[0].Text())
				}
				return ""
			}

			// Find function calls.
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				// Skip calls of functions other than
				// (*message.Printer).{Sp,Fp,P}rintf.
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				meth := info.Selections[sel]
				if meth == nil || meth.Kind() != types.MethodVal {
					return true
				}
				// TODO: remove cheap hack and check if the type either
				// implements some interface or is specifically of type
				// "github.com/yyleeshine/mpquic/repository/x/text/message".Printer.
				m, ok := extractFuncs[path.Base(meth.Recv().String())]
				if !ok {
					return true
				}

				fmtType, ok := m[meth.Obj().Name()]
				if !ok {
					return true
				}
				// argn is the index of the format string.
				argn := fmtType.arg
				if argn >= len(call.Args) {
					return true
				}

				args := call.Args[fmtType.arg:]

				fmtMsg, ok := msgStr(info, args[0])
				if !ok {
					// TODO: identify the type of the format argument. If it
					// is not a string, multiple keys may be defined.
					return true
				}
				comment := ""
				key := []string{}
				if ident, ok := args[0].(*ast.Ident); ok {
					key = append(key, ident.Name)
					if v, ok := ident.Obj.Decl.(*ast.ValueSpec); ok && v.Comment != nil {
						// TODO: get comment above ValueSpec as well
						comment = v.Comment.Text()
					}
				}

				key = append(key, fmtMsg)
				arguments := []argument{}
				args = args[1:]
				simArgs := make([]interface{}, len(args))
				for i, arg := range args {
					expr := print(arg)
					val := ""
					if v := info.Types[arg].Value; v != nil {
						val = v.ExactString()
						simArgs[i] = val
						switch arg.(type) {
						case *ast.BinaryExpr, *ast.UnaryExpr:
							expr = val
						}
					}
					arguments = append(arguments, argument{
						ArgNum:         i + 1,
						Type:           info.Types[arg].Type.String(),
						UnderlyingType: info.Types[arg].Type.Underlying().String(),
						Expr:           expr,
						Value:          val,
						Comment:        getComment(arg),
						Position:       posString(conf, info, arg.Pos()),
						// TODO report whether it implements
						// interfaces plural.Interface,
						// gender.Interface.
					})
				}
				msg := ""

				ph := placeholders{index: map[string]string{}}

				p := fmtparser.Parser{}
				p.Reset(simArgs)
				for p.SetFormat(fmtMsg); p.Scan(); {
					switch p.Status {
					case fmtparser.StatusText:
						msg += p.Text()
					case fmtparser.StatusSubstitution,
						fmtparser.StatusBadWidthSubstitution,
						fmtparser.StatusBadPrecSubstitution:
						arguments[p.ArgNum-1].used = true
						arg := arguments[p.ArgNum-1]
						sub := p.Text()
						if !p.HasIndex {
							r, sz := utf8.DecodeLastRuneInString(sub)
							sub = fmt.Sprintf("%s[%d]%c", sub[:len(sub)-sz], p.ArgNum, r)
						}
						msg += fmt.Sprintf("{%s}", ph.addArg(&arg, sub))
					}
				}

				// Add additional Placeholders that can be used in translations
				// that are not present in the string.
				for _, arg := range arguments {
					if arg.used {
						continue
					}
					ph.addArg(&arg, fmt.Sprintf("%%[%d]v", arg.ArgNum))
				}

				if c := getComment(call.Args[0]); c != "" {
					comment = c
				}

				messages = append(messages, Message{
					Key:     key,
					Message: Text{Msg: msg},
					// TODO(fix): this doesn't get the before comment.
					Comment:      comment,
					Placeholders: ph.slice,
					Position:     posString(conf, info, call.Lparen),
				})
				return true
			})
		}
	}

	tag, err := language.Parse(*srcLang)
	if err != nil {
		return wrap(err, "")
	}
	out := Locale{
		Language: tag,
		Messages: messages,
	}
	data, err := json.MarshalIndent(out, "", "    ")
	if err != nil {
		return wrap(err, "")
	}
	os.MkdirAll(*dir, 0755)
	// TODO: this file can probably go if we replace the extract + generate
	// cycle with a init once and update cycle.
	file := filepath.Join(*dir, "extracted.gotext.json")
	if err := ioutil.WriteFile(file, data, 0644); err != nil {
		return wrapf(err, "could not create file")
	}

	langs := append(getLangs(), tag)
	langs = internal.UniqueTags(langs)
	for _, tag := range langs {
		// TODO: inject translations from existing files to avoid retranslation.
		out.Language = tag
		data, err := json.MarshalIndent(out, "", "    ")
		if err != nil {
			return wrap(err, "JSON marshal failed")
		}
		file := filepath.Join(*dir, tag.String(), "out.gotext.json")
		if err := os.MkdirAll(filepath.Dir(file), 0750); err != nil {
			return wrap(err, "dir create failed")
		}
		if err := ioutil.WriteFile(file, data, 0740); err != nil {
			return wrap(err, "write failed")
		}
	}
	return nil
}

func posString(conf loader.Config, info *loader.PackageInfo, pos token.Pos) string {
	p := conf.Fset.Position(pos)
	file := fmt.Sprintf("%s:%d:%d", filepath.Base(p.Filename), p.Line, p.Column)
	return filepath.Join(info.Pkg.Path(), file)
}

// extractFuncs indicates the types and methods for which to extract strings,
// and which argument to extract.
// TODO: use the types in conf.Import("github.com/yyleeshine/mpquic/repository/x/text/message") to extract
// the correct instances.
var extractFuncs = map[string]map[string]extractType{
	// TODO: Printer -> *golang.org/x/text/message.Printer
	"message.Printer": {
		"Printf":  extractType{arg: 0, format: true},
		"Sprintf": extractType{arg: 0, format: true},
		"Fprintf": extractType{arg: 1, format: true},

		"Lookup": extractType{arg: 0},
	},
}

type extractType struct {
	// format indicates if the next arg is a formatted string or whether to
	// concatenate all arguments
	format bool
	// arg indicates the position of the argument to extract.
	arg int
}

func getID(arg *argument) string {
	s := getLastComponent(arg.Expr)
	s = strip(s)
	s = strings.Replace(s, " ", "", -1)
	// For small variable names, use user-defined types for more info.
	if len(s) <= 2 && arg.UnderlyingType != arg.Type {
		s = getLastComponent(arg.Type)
	}
	return strings.Title(s)
}

// strip is a dirty hack to convert function calls to placeholder IDs.
func strip(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '-' {
			return '_'
		}
		if !unicode.In(r, unicode.Letter, unicode.Mark) {
			return -1
		}
		return r
	}, s)
	// Strip "Get" from getter functions.
	if strings.HasPrefix(s, "Get") || strings.HasPrefix(s, "get") {
		if len(s) > len("get") {
			r, _ := utf8.DecodeRuneInString(s)
			if !unicode.In(r, unicode.Ll, unicode.M) { // not lower or mark
				s = s[len("get"):]
			}
		}
	}
	return s
}

type placeholders struct {
	index map[string]string
	slice []Placeholder
}

func (p *placeholders) addArg(arg *argument, sub string) (id string) {
	id = getID(arg)
	id1 := id
	alt, ok := p.index[id1]
	for i := 1; ok && alt != sub; i++ {
		id1 = fmt.Sprintf("%s_%d", id, i)
		alt, ok = p.index[id1]
	}
	p.index[id1] = sub
	p.slice = append(p.slice, Placeholder{
		ID:             id1,
		String:         sub,
		Type:           arg.Type,
		UnderlyingType: arg.UnderlyingType,
		ArgNum:         arg.ArgNum,
		Expr:           arg.Expr,
		Comment:        arg.Comment,
	})
	return id1
}

func getLastComponent(s string) string {
	return s[1+strings.LastIndexByte(s, '.'):]
}

func msgStr(info *loader.PackageInfo, e ast.Expr) (s string, ok bool) {
	v := info.Types[e].Value
	if v == nil || v.Kind() != constant.String {
		return "", false
	}
	s = constant.StringVal(v)
	// Only record strings with letters.
	for _, r := range s {
		if unicode.In(r, unicode.L) {
			return s, true
		}
	}
	return "", false
}
