%{
package parser

import (
    "bytes"
    "io/ioutil"
    "path/filepath"
)

%}
%type<stmts> block
%type<stmt>  stat
%type<stmts> elseifs
%type<expr> var
%type<namelist> namelist
%type<exprlist> exprlist
%type<exprlist> exprlistassign
%type<expr> expr
%type<expr> string
%type<expr> prefixexp
%type<expr> functioncall
%type<expr> afunctioncall
%type<exprlist> args
%type<expr> function
%type<expr> functionargnames
%type<expr> mapgen

%union {
  token  Token

  stmts    *Node
  stmt     *Node

  funcname interface{}
  funcexpr interface{}

  exprlist *Node
  expr     *Node

  namelist *Node
}

/* Reserved words */
%token<token> TAnd TAssert TBreak TContinue TDo TElse TElseIf TEnd TFalse TFor TIf TLambda TList TNil TNot TOr TReturn TRequire TSet TThen TTrue TYield

/* Literals */
%token<token> TEqeq TNeq TLsh TRsh TLte TGte TIdent TNumber TString '{' '('

/* Operators */
%left TOr
%left TAnd
%left '|' '&' '^'
%left '>' '<' TGte TLte TEqeq TNeq
%left TLsh TRsh
%left '+' '-'
%left '*' '/' '%'
%right UNARY /* not # -(unary) */
%right '~'

%% 

block: 
        {
            $$ = NewCompoundNode("chain")
            if l, ok := yylex.(*Lexer); ok {
                l.Stmts = $$
            }
        } |
        block stat {
            if $2.isIsolatedDupCall() {
                $2.Compound[2].Compound[0] = NewNumberNode("0")
            }
            $1.Compound = append($1.Compound, $2)
            $$ = $1
            if l, ok := yylex.(*Lexer); ok {
                l.Stmts = $$
            }
        } | 
        block ';' {
            $$ = $1
            if l, ok := yylex.(*Lexer); ok {
                l.Stmts = $$
            }
        }

stat:
        var '=' expr {
            $$ = NewCompoundNode("move", $1, $3)
            if len($1.Compound) > 0 {
                if c, _ := $1.Compound[0].Value.(string); c == "load" {
                    $$ = NewCompoundNode("store", $1.Compound[1], $1.Compound[2], $3)
                }
            }
            if c, _ := $1.Value.(string); c != "" && $1.Type == NTAtom {
                if a, b, s := $3.isSimpleAddSub(); a == c {
                    $3.Compound[2].Value = $3.Compound[2].Value.(float64) * s
                    $$ = NewCompoundNode("inc", $1, $3.Compound[2])
                    $$.Compound[1].Pos = $1.Pos
                } else if b == c {
                    $3.Compound[1].Value = $3.Compound[1].Value.(float64) * s
                    $$ = NewCompoundNode("inc", $1, $3.Compound[1])
                    $$.Compound[1].Pos = $1.Pos
                }
            }
            $$.Compound[0].Pos = $1.Pos
        } |
        /* 'stat = functioncal' causes a reduce/reduce conflict */
        prefixexp {
            // if _, ok := $1.(*FuncCallExpr); !ok {
            //    yylex.(*Lexer).Error("parse error")
            // } else {
            $$ = $1
            // }
        } |
        TFor expr TDo block TEnd {
            $$ = NewCompoundNode("for", $2, NewCompoundNode(), $4)
            $$.Compound[0].Pos = $1.Pos
        } |
        TFor expr ',' stat TDo block TEnd {
            $$ = NewCompoundNode("for", $2, NewCompoundNode("chain", $4), $6)
            $$.Compound[0].Pos = $1.Pos
        } |
        TLambda TIdent functionargnames block TEnd {
            funcname := NewAtomNode($2)
            $$ = NewCompoundNode("chain", NewCompoundNode("set", funcname, NewNilNode()), NewCompoundNode("move", funcname, NewCompoundNode("lambda", $3, $4)))
        } |
        TIf expr TThen block elseifs TEnd {
            $$ = NewCompoundNode("if", $2, $4, NewCompoundNode())
            $$.Compound[0].Pos = $1.Pos
            cur := $$
            for _, e := range $5.Compound {
                cur.Compound[3] = NewCompoundNode("chain", e)
                cur = e
            }
        } |
        TIf expr TThen block elseifs TElse block TEnd {
            $$ = NewCompoundNode("if", $2, $4, NewCompoundNode())
            $$.Compound[0].Pos = $1.Pos
            cur := $$
            for _, e := range $5.Compound {
                cur.Compound[3] = NewCompoundNode("chain", e)
                cur = e
            }
            cur.Compound[3] = $7
        } |
        TSet namelist '=' exprlist {
            $$ = NewCompoundNode("chain")
            for i, name := range $2.Compound {
                var e *Node
                if i < len($4.Compound) {
                    e = $4.Compound[i]
                } else {
                    e = $4.Compound[len($4.Compound) - 1]
                }
                c := NewCompoundNode("set", name, e)
                name.Pos, e.Pos = $1.Pos, $1.Pos
                c.Compound[0].Pos = $1.Pos
                $$.Compound = append($$.Compound, c)
            }
        } |
        TReturn {
            $$ = NewCompoundNode("ret")
            $$.Compound[0].Pos = $1.Pos
        } |
        TReturn expr {
            if $2.isIsolatedDupCall() {
                if h, _ := $2.Compound[2].Compound[2].Value.(float64); h == 1 {
                    $2.Compound[2].Compound[2] = NewNumberNode("2")
                }
            }
            $$ = NewCompoundNode("ret", $2)
            $$.Compound[0].Pos = $1.Pos
        } |
        TYield {
            $$ = NewCompoundNode("yield")
            $$.Compound[0].Pos = $1.Pos
        } |
        TYield expr {
            $$ = NewCompoundNode("yield", $2)
            $$.Compound[0].Pos = $1.Pos
        } |
        TBreak  {
            $$ = NewCompoundNode("break")
            $$.Compound[0].Pos = $1.Pos
        } |
        TContinue  {
            $$ = NewCompoundNode("continue")
            $$.Compound[0].Pos = $1.Pos
        } |
        TAssert expr {
            $$ = NewCompoundNode("assert", $2)
            $$.Compound[0].Pos = $2.Pos
        } |
        TRequire TString {
            path := filepath.Dir($1.Pos.Source)
            path = filepath.Join(path, $2.Str)
            filename := filepath.Base($2.Str)
            filename = filename[:len(filename) - len(filepath.Ext(filename))]

            code, err := ioutil.ReadFile(path)
            if err != nil {
                yylex.(*Lexer).Error(err.Error())
            }
            n, err := Parse(bytes.NewReader(code), path)
            if err != nil {
                yylex.(*Lexer).Error(err.Error())
            }

            // now the required code is loaded, for naming scope we will wrap them into a closure
            cls := NewCompoundNode("lambda", NewCompoundNode(), n)
            call := NewCompoundNode("call", cls, NewCompoundNode())
            $$ = NewCompoundNode("set", filename, call)
        }

elseifs: 
        {
            $$ = NewCompoundNode()
        } | 
        elseifs TElseIf expr TThen block {
            $$.Compound = append($1.Compound, NewCompoundNode("if", $3, $5, NewCompoundNode()))
        }

var:
        TIdent {
            $$ = NewAtomNode($1)
        } |
        prefixexp '[' expr ']' {
            $$ = NewCompoundNode("load", $1, $3)
        } |
        prefixexp '.' TIdent {
            $$ = NewCompoundNode("load", $1, NewStringNode($3.Str))
        }

namelist:
        TIdent {
            $$ = NewCompoundNode($1.Str)
        } | 
        namelist ',' TIdent {
            $1.Compound = append($1.Compound, NewAtomNode($3))
            $$ = $1
        }

exprlist:
        expr {
            $$ = NewCompoundNode($1)
        } |
        exprlist ',' expr {
            $1.Compound = append($1.Compound, $3)
            $$ = $1
        }

exprlistassign:
        expr '=' expr {
            $$ = NewCompoundNode($1, $3)
        } |
        exprlistassign ',' expr '=' expr {
            $1.Compound = append($1.Compound, $3, $5)
            $$ = $1
        }

expr:
        TNil {
            $$ = NewNilNode()
            $$.Pos = $1.Pos
        } | 
        TFalse {
            $$ = NewCompoundNode("false")
            $$.Pos = $1.Pos
        } | 
        TTrue {
            $$ = NewCompoundNode("true")
            $$.Pos = $1.Pos
        } | 
        TNumber {
            $$ = NewNumberNode($1.Str)
            $$.Pos = $1.Pos
        } |
        function {
            $$ = $1
        } |
        mapgen {
            $$ = $1
        } | 
        prefixexp {
            $$ = $1
        } |
        string {
            $$ = $1
        } |
        expr TOr expr {
            $$ = NewCompoundNode("or", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr TAnd expr {
            $$ = NewCompoundNode("and", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr '>' expr {
            $$ = NewCompoundNode("<", $3,$1)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr '<' expr {
            $$ = NewCompoundNode("<", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr TGte expr {
            $$ = NewCompoundNode("<=", $3,$1)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr TLte expr {
            $$ = NewCompoundNode("<=", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr TEqeq expr {
            $$ = NewCompoundNode("eq", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr TNeq expr {
            $$ = NewCompoundNode("neq", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr '+' expr {
            $$ = NewCompoundNode("+", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr '-' expr {
            $$ = NewCompoundNode("-", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr '*' expr {
            $$ = NewCompoundNode("*", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr '/' expr {
            $$ = NewCompoundNode("/", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr '%' expr {
            $$ = NewCompoundNode("%", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr '^' expr {
            $$ = NewCompoundNode("^", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr TLsh expr {
            $$ = NewCompoundNode("<<", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr TRsh expr {
            $$ = NewCompoundNode(">>", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr '|' expr {
            $$ = NewCompoundNode("|", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        expr '&' expr {
            $$ = NewCompoundNode("&", $1,$3)
            $$.Compound[0].Pos = $1.Pos
        } |
        '-' expr %prec UNARY {
            $$ = NewCompoundNode("-", NewNumberNode("0"), $2)
            $$.Compound[0].Pos = $2.Pos
        } |
        '~' expr %prec UNARY {
            $$ = NewCompoundNode("~", $2)
            $$.Compound[0].Pos = $2.Pos
        } |
        TNot expr %prec UNARY {
            $$ = NewCompoundNode("not", $2)
            $$.Compound[0].Pos = $2.Pos
        }

string: 
        TString {
            $$ = NewStringNode($1.Str)
            $$.Pos = $1.Pos
        } 

prefixexp:
        var {
            $$ = $1
        } |
        afunctioncall {
            $$ = $1
        } |
        functioncall {
            $$ = $1
        } |
        '(' expr ')' {
            $$ = $2
        }

afunctioncall:
        '(' functioncall ')' {
            $$ = $2
        }

functioncall:
        prefixexp args {
            switch c, _ := $1.Value.(string); c {
            case "dup":
                switch len($2.Compound) {
                case 0:
                    $$ = NewCompoundNode("call", $1, NewCompoundNode(NewNumberNode("1"), NewNumberNode("1"), NewNumberNode("1")))
                case 1:
                    $$ = NewCompoundNode("call", $1, NewCompoundNode(NewNumberNode("1"), $2.Compound[0], NewNumberNode("0")))
                default:
                    p := $2.Compound[1]
                    if p.Type != NTCompound && p.Type != NTAtom {
                        yylex.(*Lexer).Error("the second argument of dup must be a closure")
                    }
                    $$ = NewCompoundNode("call", $1, NewCompoundNode(NewNumberNode("1"), $2.Compound[0], p))
                }
            case "error":
                if len($2.Compound) == 0 {
                    $$ = NewCompoundNode("call", $1, NewCompoundNode(NewNilNode()))
                } else {
                    $$ = NewCompoundNode("call", $1, $2)
                }
            case "typeof":
                switch len($2.Compound) {
                case 0:
                    yylex.(*Lexer).Error("typeof takes at least 1 argument")
                case 1:
                    $$ = NewCompoundNode("call", $1, NewCompoundNode($2.Compound[0], NewNumberNode("255")))
                default:
                    switch x, _ := $2.Compound[1].Value.(string); x {
                    case "nil":
                        $$ = NewCompoundNode("call", $1, NewCompoundNode($2.Compound[0], NewNumberNode("0")))
                    case "number":
                        $$ = NewCompoundNode("call", $1, NewCompoundNode($2.Compound[0], NewNumberNode("1")))
                    case "string":
                        $$ = NewCompoundNode("call", $1, NewCompoundNode($2.Compound[0], NewNumberNode("2")))
                    case "map":
                        $$ = NewCompoundNode("call", $1, NewCompoundNode($2.Compound[0], NewNumberNode("3")))
                    case "closure":
                        $$ = NewCompoundNode("call", $1, NewCompoundNode($2.Compound[0], NewNumberNode("4")))
                    case "generic":
                        $$ = NewCompoundNode("call", $1, NewCompoundNode($2.Compound[0], NewNumberNode("5")))
                    default:
                        $$ = NewCompoundNode("call", $1, NewCompoundNode($2.Compound[0], $2.Compound[1]))
                    }
                }
            case "len":
                switch len($2.Compound) {
                case 0:
                    yylex.(*Lexer).Error("len takes 1 argument")
                default:
                    $$ = NewCompoundNode("call", $1, $2)
                }
            default:
                $$ = NewCompoundNode("call", $1, $2)
            }
        }

args:
        '(' ')' {
            if yylex.(*Lexer).PNewLine {
               yylex.(*Lexer).TokenError($1, "ambiguous syntax (function call x new statement)")
            }
            $$ = NewCompoundNode()
        } |
        '(' exprlist ')' {
            if yylex.(*Lexer).PNewLine {
               yylex.(*Lexer).TokenError($1, "ambiguous syntax (function call x new statement)")
            }
            $$ = $2
        }

function:
        TLambda functionargnames block TEnd {
            $$ = NewCompoundNode("lambda", $2, $3)
            $$.Compound[0].Pos = $1.Pos
        }

functionargnames:
        '(' ')' {
            $$ = NewCompoundNode()
        } |
        '(' namelist ')' {
            $$ = $2
        }

mapgen:
        '{' '}' {
            $$ = NewCompoundNode("map", NewCompoundNode())
            $$.Compound[0].Pos = $1.Pos
        } |
        '{' exprlistassign '}' {
            $$ = NewCompoundNode("map", $2)
            $$.Compound[0].Pos = $1.Pos
        } |
        '{' exprlist '}' {
            table := NewCompoundNode()
            for i, v := range $2.Compound {
                table.Compound = append(table.Compound, 
                    &Node{ Type:  NTNumber, Value: float64(i) },
                    v)
            }
            $$ = NewCompoundNode("map", table)
            $$.Compound[0].Pos = $1.Pos
        }

%%

func TokenName(c int) string {
	if c >= TAnd && c-TAnd < len(yyToknames) {
		if yyToknames[c-TAnd] != "" {
			return yyToknames[c-TAnd]
		}
	}
    return string([]byte{byte(c)})
}

