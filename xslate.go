//go:generate stringer -type=NodeType node/node.go

/*
Package xslate is the main interface to the Go version of Xslate.

Xslate is an extremely powerful template engine, based on Perl5's Text::Xslate
module (http://xslate.org/). Xslate uses a virtual machine to execute
pre-compiled template bytecode, which gives its flexibility while maitaining
a very fast execution speed.

You may be thinking "What? Go already has text/template and html/template!".
You are right in that this is another added complexity, but the major
difference is that Xslate assumes that your template data resides outside
of your go application (e.g. on the file system), and properly handles
updates to those templates -- you don't have to recompile your application
or write freshness checks yourselve to get the updates reflected automatically.

Xslate does all that, and also tries its best to keep things fast by
creating memory caches and file-based caches.

It also supports a template syntax known as TTerse, which is a simplified
version of the famous Template-Toolkit (http://www.template-toolkit.org)
syntax, which is a full-blown language on its own that allows you to create
flexible, rich templates.

The simplest way to use Xslate is to prepare a directory with Xslate templates
(say, "/path/to/tempalte"), and do something like the following:

  tx, _ := xslate.New(xslate.Args {
    "Loader": xslate.Args {
      "LoadPaths": []string { "/path/to/templates" },
    },
  })
  output, _ := tx.Render("main.tx", xslate.Vars { ... })
  fmt.Println(output)

By default Xslate loads templates from the filesystem AND caches the generated
compiled bytecode into a temporary location so that the second time the same
template is called, no parsing is required.

Note that RenderString() DOES NOT CACHE THE GENERATED BYTECODE. This has
significant effect on performance if you repeatedly call the same template.
It is strongly recommended that you use the caching layer to boost performance.

*/
package xslate

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"runtime"
	"strconv"

	"github.com/lestrrat/go-xslate/compiler"
	"github.com/lestrrat/go-xslate/internal/rbpool"
	"github.com/lestrrat/go-xslate/loader"
	"github.com/lestrrat/go-xslate/parser"
	"github.com/lestrrat/go-xslate/parser/kolonish"
	"github.com/lestrrat/go-xslate/parser/tterse"
	"github.com/lestrrat/go-xslate/vm"
	"github.com/pkg/errors"
)

// Debug enables debug output. This can be toggled by setting XSLATE_DEBUG
// environment variable.
var Debug = false

func init() {
	tmp := os.Getenv("XSLATE_DEBUG")
	boolVar, err := strconv.ParseBool(tmp)
	if err == nil {
		Debug = boolVar
	}
}

// Vars is an alias to vm.Vars, declared so that you (the end user) does
// not have to import two packages just to use Xslate
type Vars vm.Vars

// Xslate is the main package containing all the goodies to execute and
// render an Xslate template
type Xslate struct {
	Flags    int32
	VM       *vm.VM
	Compiler compiler.Compiler
	Parser   parser.Parser
	Loader   loader.ByteCodeLoader
	// XXX Need to make syntax pluggable
}

// ConfigureArgs is the interface to be passed to `Configure()` method.
// It just needs to be able to access fields by name like a map
type ConfigureArgs interface {
	Get(string) (interface{}, bool)
}

// Args is the concret type that implements `ConfigureArgs`. Normally
// this is all you need to pass to `New()`
type Args map[string]interface{}

// DefaultCompiler sets up and assigns the default compiler to be used by
// Xslate. Given an unconfigured Xslate instance and arguments, sets up
// the compiler of said Xslate instance. Current implementation
// just uses compiler.New()
func DefaultCompiler(tx *Xslate, args Args) error {
	tx.Compiler = compiler.New()
	return nil
}

// DefaultParser sets up and assigns the default parser to be used by Xslate.
func DefaultParser(tx *Xslate, args Args) error {
	syntax, ok := args.Get("Syntax")
	if !ok {
		syntax = "TTerse"
	}

	switch syntax {
	case "TTerse":
		tx.Parser = tterse.New()
	case "Kolon", "Kolonish":
		tx.Parser = kolonish.New()
	default:
		return errors.New("sytanx '" + syntax.(string) + "' is not available")
	}
	return nil
}

// DefaultLoader sets up and assigns the default loader to be used by Xslate.
func DefaultLoader(tx *Xslate, args Args) error {
	var tmp interface{}

	tmp, ok := args.Get("CacheDir")
	if !ok {
		tmp, _ = ioutil.TempDir("", "go-xslate-cache-")

	}
	cacheDir := tmp.(string)

	tmp, ok = args.Get("LoadPaths")
	if !ok {
		cwd, _ := os.Getwd()
		tmp = []string{cwd}
	}
	paths := tmp.([]string)

	cache, err := loader.NewFileCache(cacheDir)
	if err != nil {
		return err
	}
	fetcher, err := loader.NewFileTemplateFetcher(paths)
	if err != nil {
		return err
	}

	tmp, ok = args.Get("CacheLevel")
	if !ok {
		tmp = 1
	}
	cacheLevel := tmp.(int)
	tx.Loader = loader.NewCachedByteCodeLoader(cache, loader.CacheStrategy(cacheLevel), fetcher, tx.Parser, tx.Compiler)
	return nil
}

// DefaultVM sets up and assigns the default VM to be used by Xslate
func DefaultVM(tx *Xslate, args Args) error {
	dvm := vm.NewVM()
	dvm.Loader = tx.Loader
	tx.VM = dvm
	return nil
}

// Get retrieves the value assigned to `key`
func (args Args) Get(key string) (interface{}, bool) {
	ret, ok := args[key]
	return ret, ok
}

func (tx *Xslate) configureGeneric(configuror interface{}, args Args) error {
	ref := reflect.ValueOf(configuror)
	switch ref.Type().Kind() {
	case reflect.Func:
		// If this is a function, it better take our Xslate instance as the
		// sole argument, and initialize it as it pleases
		if ref.Type().NumIn() != 2 && (ref.Type().In(0).Name() != "Xslate" || ref.Type().In(1).Name() != "Args") {
			panic(fmt.Sprintf(`Expected function initializer "func (tx *Xslate)", but instead of %s`, ref.Type().String()))
		}
		cb := configuror.(func(*Xslate, Args) error)
		err := cb(tx, args)
		return err
	}
	return errors.New("error: Bad configurator")
}

// Configure is called automatically from `New()` to configure the xslate
// instance from arguments
func (tx *Xslate) Configure(args ConfigureArgs) error {
	// The compiler currently does not have any configurable options, but
	// one may want to replace the entire compiler struct
	defaults := map[string]func(*Xslate, Args) error{
		"Compiler": DefaultCompiler,
		"Parser":   DefaultParser,
		"Loader":   DefaultLoader,
		"VM":       DefaultVM,
	}

	for _, key := range []string{"Parser", "Compiler", "Loader", "VM"} {
		configKey := "Configure" + key
		configuror, ok := args.Get(configKey)
		if !ok {
			configuror = defaults[key]
		}

		args, ok := args.Get(key)
		if !ok {
			args = Args{}
		}

		err := tx.configureGeneric(configuror, args.(Args))
		if err != nil {
			return err
		}
	}

	// Configure Functions
	if funcs, ok := args.Get("Functions"); ok {
		tx.VM.SetFunctions(vm.Vars(funcs.(Args)))
	}

	if Debug {
		tx.DumpAST(true)
		tx.DumpByteCode(true)
	}

	return nil
}

// New creates a new Xslate instance. If called without any arguments,
// creates a new Xslate instance using all default settings.
//
// To pass parameters, use `xslate.Vars`
//
// Possible Options:
//    * ConfigureLoader: Callback to setup the Loader. See DefaultLoader
//    * ConfigureParser: Callback to setup the Parser. See DefaultParser
//    * ConfigureCompiler: Callback to setup the Compiler. See DefaultCompiler
//    * ConfigureVM: Callback to setup the Virtual Machine. See DefaultVM
//    * Parser: Arbitrary arguments passed to ConfigureParser function
//    * Loader: Arbitrary arguments passed to ConfigureLoader function
//    * Compiler: Arbitrary arguments passed to ConfigureCompiler function
//    * VM: Arbitrary arguments passed to ConfigureVM function
func New(args ...Args) (*Xslate, error) {
	tx := &Xslate{}

	// We jump through hoops because there are A LOT of configuration options
	// but most of them only need to use the default values
	if len(args) <= 0 {
		args = []Args{Args{}}
	}
	err := tx.Configure(args[0])
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// DumpAST sets the flag to dump the abstract syntax tree after parsing the
// template. Use of this method is only really useful if you know the internal
// repreentation of the templates
func (tx *Xslate) DumpAST(b bool) {
	tx.Loader.DumpAST(b)
}

// DumpByteCode sets the flag to dump the bytecode after compiling the
// template. Use of this method is only really useful if you know the internal
// repreentation of the templates
func (tx *Xslate) DumpByteCode(b bool) {
	tx.Loader.DumpByteCode(b)
}

// Render loads the template specified by the given name string.
// By default Xslate looks for files in the local file system, and caches
// the generated bytecode too.
//
// If you wish to, for example, load the templates from a database, you can
// change the generated loader object by providing a `ConfigureLoader`
// parameter in the xslate.New() function:
//
//    xslate.New(Args {
//      "ConfigureLoader": func(tx *Xslate, args Args) {
//        tx.Loader = .... // your custom loader
//      },
//    })
//
// `Render()` returns the resulting text from processing the template.
// `err` is nil on success, otherwise it contains an `error` value.
func (tx Xslate) Render(name string, vars Vars) (string, error) {
	buf := rbpool.Get()
	defer rbpool.Release(buf)

	err := tx.RenderInto(buf, name, vars)
	if err != nil {
		return "", errors.Wrap(err, "failed to render template")
	}

	return buf.String(), nil
}

// RenderString takes a string argument and treats it as the template
// content. Like `Render()`, this template is parsed and compiled. Because
// there's no way to establish template "freshness", the resulting bytecode
// from `RenderString()` is not cached for reuse.
//
// If you *really* want to change this behavior, it's not impossible to
// bend Xslate's Loader mechanism to cache strings as well, but the main
// Xslate library will probably not adopt this feature.
func (tx *Xslate) RenderString(template string, vars Vars) (string, error) {
	_, file, line, _ := runtime.Caller(1)
	bc, err := tx.Loader.LoadString(fmt.Sprintf("%s:%d", file, line), template)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse template string")
	}

	buf := rbpool.Get()
	defer rbpool.Release(buf)

	tx.VM.Run(bc, vm.Vars(vars), buf)
	return buf.String(), nil
}

// RenderInto combines Render() and writing its results into an io.Writer.
// This is a convenience method for frameworks providing a Writer interface,
// such as net/http's ServeHTTP()
func (tx *Xslate) RenderInto(w io.Writer, template string, vars Vars) error {
	bc, err := tx.Loader.Load(template)
	if err != nil {
		return err
	}
	tx.VM.Run(bc, vm.Vars(vars), w)
	return nil
}
