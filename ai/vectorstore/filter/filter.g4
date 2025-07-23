grammar filter;

expr        : orExpr ;
orExpr      : andExpr (('OR' | 'or') andExpr)* ;
andExpr     : notExpr (('AND' | 'and') notExpr)* ;
notExpr     : ('NOT' | 'not') notExpr | compareExpr ;
compareExpr : primary compareOp primary ;
primary     : accessExpr | literal | parenExpr | listLit ;
accessExpr  : IDENT ('[' accessKey ']')* ;
accessKey   : STRING_LIT | NUM_LIT ;
compareOp   : '==' | '!=' | '<' | '<=' | '>' | '>=' |
              ('LIKE' | 'like') | ('IN' | 'in') ;
parenExpr   : '(' expr ')' ;
listLit     : '(' litSeq ')' ;
litSeq      : (literal (',' literal)*)? ;
literal     : NUM_LIT | STRING_LIT | BOOL_LIT ;
NUM_LIT     : DEC_NUM | INT_NUM ;
DEC_NUM     : INT_NUM '.' DIGIT+ ;
INT_NUM     : DIGIT+ ;
STRING_LIT  : '\'' (ESCAPE_CHAR | NORMAL_CHAR)* '\'' ;
BOOL_LIT    : TRUE_LIT | FALSE_LIT ;
TRUE_LIT    : 'TRUE' | 'true' ;
FALSE_LIT   : 'FALSE' | 'false' ;
IDENT       : LETTER (LETTER | DIGIT | '_')* ;
fragment ESCAPE_CHAR : '\\\\' | '\\\'' | '\\"' | '\\n' | '\\t' | '\\r' ;
fragment NORMAL_CHAR : ~['] ;
fragment DIGIT       : [0-9] ;
fragment LETTER      : [a-zA-Z] ;
WS : [ \t\r\n]+ -> skip ;