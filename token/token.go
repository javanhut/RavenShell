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
	SHOW       TokenType = "SHOW"
	CLEAR      TokenType = "CLEAR"
	GREATER    TokenType = "GREATER"
	INTO       TokenType = "INTO"
	LESS       TokenType = "LESS"
	OUT        TokenType = "OUT"
	FULLSTOP   TokenType = "FULLSTOP"
	FSLASH     TokenType = "FSLASH"
	TILDE      TokenType = "TILDE"

	// Control flow keywords
	FOR      TokenType = "FOR"
	IN       TokenType = "IN"
	IF       TokenType = "IF"
	ELSE     TokenType = "ELSE"
	RANGE    TokenType = "RANGE"
	APPEND   TokenType = "APPEND"
	BREAK    TokenType = "BREAK"
	CONTINUE TokenType = "CONTINUE"
	FUNCTION TokenType = "FUNCTION"
	RETURN   TokenType = "RETURN"
	SWITCH   TokenType = "SWITCH"
	CASE     TokenType = "CASE"
	DEFAULT  TokenType = "DEFAULT"

	// Delimiters
	LBRACE   TokenType = "LBRACE"   // {
	RBRACE   TokenType = "RBRACE"   // }
	LPAREN   TokenType = "LPAREN"   // (
	RPAREN   TokenType = "RPAREN"   // )
	LBRACKET TokenType = "LBRACKET" // [
	RBRACKET TokenType = "RBRACKET" // ]
	COMMA    TokenType = "COMMA"    // ,
	COLON    TokenType = "COLON"    // :

	// Operators
	ASSIGN   TokenType = "ASSIGN"   // =
	PLUS     TokenType = "PLUS"     // +
	MINUS    TokenType = "MINUS"    // -
	ASTERISK TokenType = "ASTERISK" // *
	PERCENT  TokenType = "PERCENT"  // %
	EQ       TokenType = "EQ"       // ==
	NOT_EQ   TokenType = "NOT_EQ"   // !=
	LT       TokenType = "LT"       // < (for comparisons, different from LESS for redirection)
	GT       TokenType = "GT"       // > (for comparisons, different from GREATER for redirection)
	LTE      TokenType = "LTE"      // <=
	GTE      TokenType = "GTE"      // >=

	// Logical operators
	AND   TokenType = "AND"   // &&
	OR    TokenType = "OR"    // ||
	NOT   TokenType = "NOT"   // !
	TRUE  TokenType = "TRUE"  // true
	FALSE TokenType = "FALSE" // false

	// Regex
	REGEX_MATCH TokenType = "REGEX_MATCH" // =~
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
	"output": OUTPUT,
	"print":  PRINT,
	"show":   SHOW,
	"clear":  CLEAR,
	"for":    FOR,
	"in":     IN,
	"if":     IF,
	"else":   ELSE,
	"range":    RANGE,
	"append":   APPEND,
	"break":    BREAK,
	"continue": CONTINUE,
	"fn":       FUNCTION,
	"func":     FUNCTION,
	"return":   RETURN,
	"switch":   SWITCH,
	"match":    SWITCH,
	"case":     CASE,
	"default":  DEFAULT,
	"true":     TRUE,
	"false":    FALSE,
}
