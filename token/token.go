package token

type TokenType string

type Token struct {
	Type    TokenType
	Literal string
}

const (
	// KEYWORDS
	EOF        TokenType = "EOF"
	ILLEGAL    TokenType = "ILLEGAL"
	LIST       TokenType = "LIST"
	REMOVE     TokenType = "REMOVE"
	CHANGEDIR  TokenType = "CHANGEDIR"
	REMOVEDIR  TokenType = "REMOVEDIR"
	MAKEDIR    TokenType = "MAKEDIR"
	WHOAMI     TokenType = "WHOAMI"
	CURRENTDIR TokenType = "CURRENTDIR"
	MAKEFILE   TokenType = "MAKEFILE"
	OUTPUT     TokenType = "OUTPUT"
	IDENT      TokenType = "IDENTIFER"
	INTEGER    TokenType = "INTEGER"
	STRING     TokenType = "STRING"
	PIPE       TokenType = "PIPE"
	DOLLAR     TokenType = "DOLLAR"
	PRINT      TokenType = "PRINT"
	GREATER    TokenType = "GREATER"
	INTO       TokenType = "INTO"
	LESS       TokenType = "LESS"
	OUT        TokenType = "OUT"
	FULLSTOP   TokenType = "FULLSTOP"
	FSLASH		 TokenType = "FSLASH"
)

var TokenMap = map[string]TokenType{
	"ls":     LIST,
	"rm":     REMOVE,
	"mkdir":  MAKEDIR,
	"rmdir":  REMOVEDIR,
	"cd":     CHANGEDIR,
	"cwd":    CURRENTDIR,
	"whoami": WHOAMI,
	"mkfile": MAKEFILE,
	"out": OUTPUT,
	"print":  PRINT,
}
