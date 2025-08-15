" Vim syntax file for OctoChan
" Language: OctoChan
" Maintainer: OctoChan Team
" Latest Revision: 2025-01-15

if exists("b:current_syntax")
  finish
endif

" Keywords
syn keyword octoKeyword if else for while in required_role
syn keyword octoFunction create_alias run_scenario list_scenarios has_permission
syn keyword octoBuiltin print system_info user_info apply_diff filter count compile_native
syn keyword octoBoolean true false null

" Comments
syn match octoComment "#.*$"

" Strings
syn region octoString start='"' end='"' contains=octoEscape
syn match octoEscape contained "\\."

" Numbers
syn match octoNumber "\<\d\+\>"
syn match octoFloat "\<\d\+\.\d\+\>"

" Variables
syn match octoVariable "\$[a-zA-Z_][a-zA-Z0-9_]*"
syn match octoAssignment "[a-zA-Z_][a-zA-Z0-9_]*\s*="

" Operators
syn match octoPipeline "|>"
syn match octoOperator "[=!<>]=\|[<>+\-*/=]"

" Roles
syn match octoRole '"\w\+"' contained containedin=octoRoleDecl
syn match octoRoleDecl "required_role:\s*\".*\""

" Highlighting
hi def link octoKeyword Keyword
hi def link octoFunction Function
hi def link octoBuiltin Function
hi def link octoBoolean Boolean
hi def link octoComment Comment
hi def link octoString String
hi def link octoEscape SpecialChar
hi def link octoNumber Number
hi def link octoFloat Float
hi def link octoVariable Identifier
hi def link octoAssignment Identifier
hi def link octoPipeline Operator
hi def link octoOperator Operator
hi def link octoRole String
hi def link octoRoleDecl PreProc

let b:current_syntax = "octochan"