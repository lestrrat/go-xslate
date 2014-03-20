/*
xslate is an extremely powerful template engine, based on Perl5's Text::Xslate
module. Xslate uses a virtual machine to execute pre-compiled template bytecode,
which gives its flexibility while maitaining a very fast execution speed.

Note that RenderString() DOES NOT CACHE THE GENERATED BYTECODE. This has
significant effect on performance if you repeatedly call the same template

*/
package xslate

import (
  "errors"
  "fmt"
  "io/ioutil"
  "os"
  "reflect"

  "github.com/lestrrat/go-xslate/compiler"
  "github.com/lestrrat/go-xslate/loader"
  "github.com/lestrrat/go-xslate/parser"
  "github.com/lestrrat/go-xslate/parser/tterse"
  "github.com/lestrrat/go-xslate/vm"
)

type Vars vm.Vars
type Xslate struct {
  Flags    int32
  Vm       *vm.VM
  Compiler compiler.Compiler
  Parser   parser.Parser
  Loader   loader.ByteCodeLoader
  // XXX Need to make syntax pluggable
}

type ConfigureArgs interface {
  Get(string) (interface {}, bool)
}

type Args map[string]interface {}

// Given an unconfigured Xslate instance and arguments, sets up
// the compiler of said Xslate instance. Current implementation
// just uses compiler.New()
func DefaultCompiler(tx *Xslate, args Args) error {
  tx.Compiler = compiler.New()
  return nil
}

func DefaultParser(tx *Xslate, args Args) error {
  syntax, ok := args.Get("Syntax")
  if ! ok {
    syntax = "TTerse"
  }

  switch syntax {
  case "TTerse":
    tx.Parser = tterse.New()
  default:
    return errors.New(fmt.Sprintf("Syntax '%s' not available", syntax))
  }
  return nil
}

func DefaultLoader(tx *Xslate, args Args) error {
  var tmp interface {}

  tmp, ok := args.Get("CacheDir")
  if !ok {
    tmp, _ = ioutil.TempDir("", "go-xslate-cache-")
  }
  cacheDir := tmp.(string)

  tmp, ok = args.Get("LoadPaths")
  if !ok {
    cwd, _ := os.Getwd()
    tmp = []string { cwd }
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
  tx.Loader = loader.NewCachedByteCodeLoader(cache, fetcher, tx.Parser, tx.Compiler)
  return nil
}

func DefaultVm(tx *Xslate, args Args) error {
  tx.Vm = vm.NewVM()
  tx.Vm.Loader = tx.Loader
  return nil
}

func (args Args) Get(key string) (interface {}, bool) {
  ret, ok := args[key]
  return ret, ok
}

func (tx *Xslate) configureGeneric(configuror interface {}, args Args) error {
  ref := reflect.ValueOf(configuror)
  switch ref.Type().Kind() {
  case reflect.Func:
    // If this is a function, it better take our Xslate instance as the
    // sole argument, and initialize it as it pleases
    if ref.Type().NumIn() != 2 && (ref.Type().In(0).Name() != "Xslate" || ref.Type().In(1).Name() != "Args") {
      panic(fmt.Sprintf(`Expected function initializer "func (tx *Xslate ", but instead of %s`, ref.Type.String()))
    }
    cb := configuror.(func(*Xslate, Args) error)
    err := cb(tx, args)
    return err
  }
  return errors.New("Bad configurator")
}

func (tx *Xslate) Configure(args ConfigureArgs) error {
  // The compiler currently does not have any configurable options, but
  // one may want to replace the entire compiler struct
  defaults := map[string]func(*Xslate, Args) error {
    "Compiler": DefaultCompiler,
    "Parser":   DefaultParser,
    "Loader":   DefaultLoader,
    "Vm":       DefaultVm,
  }

  for _, key := range []string { "Parser", "Compiler", "Loader", "Vm" } {
    configKey := "Configure" + key
    configuror, ok := args.Get(configKey);
    if !ok {
      configuror = defaults[key]
    }

    args, ok := args.Get(key)
    if !ok {
      args = Args {}
    }

    err := tx.configureGeneric(configuror, args.(Args))
    if err != nil {
      return err
    }
  }

  return nil
}

func New(args ...Args) (*Xslate, error) {
  tx := &Xslate {}

  // We jump through hoops because there are A LOT of configuration options
  // but most of them only need to use the default values
  if len(args) <= 0 {
    args = []Args { Args {} }
  }
  err := tx.Configure(args[0])
  if err != nil {
    return nil, err
  }
  return tx, nil
}

func (tx *Xslate) DumpAST(b bool) {
  tx.Loader.DumpAST(b)
}

func (tx *Xslate) DumpByteCode(b bool) {
  tx.Loader.DumpByteCode(b)
}

func (x *Xslate) Render(name string, vars Vars) (string, error) {
  bc, err := x.Loader.Load(name)
  if err != nil {
    return "", err
  }
  x.Vm.Run(bc, vm.Vars(vars))
  return x.Vm.OutputString()
}

func (x *Xslate) RenderString(template string, vars Vars) (string, error) {
  bc, err := x.Loader.LoadString(template)
  if err != nil {
    return "", err
  }

  x.Vm.Run(bc, vm.Vars(vars))
  return x.Vm.OutputString()
}
