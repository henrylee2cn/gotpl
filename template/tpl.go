package template

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Generate(input string, output string, option Option) error {
	tplMap := map[string]*Tpl{}

	paths, err := getFiles(input, TPL_EXT)
	if err != nil {
		return err
	}

	for i := 0; i < len(paths); i++ {
		path := paths[i]

		baseName := filepath.Base(path)
		name := strings.TrimSpace(strings.Replace(baseName, TPL_EXT, "", 1))

		tpl := &Tpl{path: path, name: name, ast: &Ast{}, tokens: []Token{}, blocks: map[string]*Ast{}, option: option}

		err := tpl.readRaw()
		if err != nil {
			fmt.Println(err)
			return err
		}

		err = tpl.checkExtends()
		if err != nil {
			fmt.Println(err)
			return err
		}

		tplMap[tpl.name] = tpl
	}

	for key, tpl := range tplMap {
		if !tpl.isRoot {
			if p, ok := tplMap[tpl.parentName]; ok {
				tplMap[key].parent = p
			} else {
				fmt.Println(tpl.parentName, "--parent not exists")
				delete(tplMap, key)
			}
		}
	}

	for _, tpl := range tplMap {
		err = tpl.generate()
		if err != nil {
			fmt.Println(err)
			return err
		}
	}

	cmd := exec.Command("gofmt", "-w", output)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Println("gofmt: ", err)
		return err
	}

	return nil
}

type Tpl struct {
	path   string
	name   string
	isRoot bool
	//	level      int
	parent     *Tpl
	parentName string
	raw        []byte
	result     string
	tokens     []Token
	ast        *Ast
	blocks     map[string]*Ast
	sections   map[string]*Ast
	generated  bool
	option     Option
}

func (tpl *Tpl) generate() error {
	if tpl.generated {
		return nil
	}

	if tpl.isRoot {
		return tpl.gen()
	} else {
		if tpl.parent != nil {
			tpl.parent.generate()
			return tpl.gen()
		}
	}

	return nil
}

func (tpl *Tpl) gen() error {
	if tpl.generated {
		return nil
	}

	err := tpl.genToken()
	if err != nil {
		return err
	}

//	fmt.Println(tpl.name, "------------------- TOKEN START -----------------")
//	for _, elem := range tpl.tokens {
//		elem.debug()
//	}
//	fmt.Println(tpl.name, "--------------------- TOKEN END -----------------\n")

	err = tpl.genAst()
	if err != nil {
		return err
	}

	fmt.Println(tpl.name, "--------------------- AST START -----------------")
	tpl.ast.debug(0, 20)
	fmt.Println(tpl.name, "--------------------- AST END -----------------\n")
	if tpl.ast.Mode != PRG {
		panic("TYPE")
	}

	err = tpl.compile()
	if err != nil {
		return err
	}

	//	err = tpl.fmt()
	//	if err != nil {
	//		return err
	//	}

	err = tpl.output()
	if err != nil {
		return err
	}

	tpl.generated = true

	return nil
}

func (tpl *Tpl) genToken() error {
	lex := &Lexer{Text: string(tpl.raw), Matches: TokenMatches}

	tokens, err := lex.Scan()
	if err != nil {
		return err
	}

	tpl.tokens = tokens

	return nil
}

func (tpl *Tpl) genAst() error {
	parser := &Parser{
		ast: tpl.ast, tokens: tpl.tokens, blocks: tpl.blocks,
		preTokens: []Token{}, initMode: UNK,
	}

	// Run() -> ast
	err := parser.Run()
	if err != nil {
		fmt.Println(err)
		return err
	}

	if !tpl.isRoot && tpl.parent != nil {
		tpl.ast = tpl.parent.ast
//		fmt.Println("len(tpl.blocks)==", len(tpl.blocks))
		if tpl.blocks != nil && len(tpl.blocks) > 0 {
			replaceBlock(tpl.ast, tpl.blocks)
		}
	}

	return nil
}

func replaceBlock(ast *Ast, blocks map[string]*Ast) {
	if ast.Children == nil || len(ast.Children) == 0 || blocks == nil || len(blocks) == 0 {
		return
	}
	for idx, c := range ast.Children {
		if a, ok := c.(*Ast); ok {
			if a.Mode == BLOCK {
//				fmt.Println("-------ast.TagName", a.TagName)
				if b, ok := blocks[a.TagName]; ok {
					fmt.Println("-------ast.TagName", a.TagName)
					ast.Children[idx] = b
					delete(blocks, ast.TagName)
				}
			}
			replaceBlock(a, blocks)
		}
	}
}

func (tpl *Tpl) compile() error {
	cp := &Compiler{
		tpl: tpl,
		ast: tpl.ast, buf: "",
		params: []string{}, parts: []Part{},
		imports: map[string]bool{},
		//		options: options,
		//		dir:     dir,
		fileName: tpl.name,
	}

	// visit() -> cp.buf
	cp.visit()

	tpl.result = cp.buf

	return nil
}

func (tpl *Tpl) readRaw() error {
	raw, err := ioutil.ReadFile(tpl.path)
	if err != nil {
		fmt.Println(err)
		return err
	}
	tpl.raw = raw

	return nil
}

func (tpl *Tpl) checkExtends() error {
	tpl.parentName = ""
	tpl.isRoot = true
	if bytes.HasPrefix(tpl.raw, []byte("@extends")) {
		lineEnd := -1
		for i := len("@extends"); i < len(tpl.raw); i++ {
			if tpl.raw[i] == '\n' {
				lineEnd = i
				break
			}
		}
		line := tpl.raw[:lineEnd+1]
		ss := strings.Split(string(line), " ")
		if len(ss) == 2 {
			parentName := strings.TrimSpace(ss[1])
			tpl.parentName = parentName
			tpl.isRoot = false
			tpl.raw = tpl.raw[lineEnd+1:]
		}
	}
	return nil
}

func (tpl *Tpl) getOutPath() string {
	return "./gen/" + tpl.name + ".go"
}

func (tpl *Tpl) fmt() error {
	cmd := exec.Command("gofmt", "-w", tpl.getOutPath())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Println("gofmt: ", err)
		return err
	}

	return nil
}

func (tpl *Tpl) output() error {
	outDir := "./gen/"
	if !exists(outDir) {
		err := os.MkdirAll(outDir, 0755)
		if err != nil {
			return err
		}
	}

	outpath := outDir + tpl.name + ".go"
	err := ioutil.WriteFile(outpath, []byte(tpl.result), 0644)
	if err != nil {
		return err
	}
	return nil
}
