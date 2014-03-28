package parser

// Parser defines the interface for Xslate parsers
type Parser interface {
  Parse(string, []byte) (*AST, error)
  ParseString(string, string) (*AST, error)
}

