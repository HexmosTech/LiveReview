package livisql

import "fmt"

// RejectionCode identifies why a generated query was refused. It is stable
// enough to assert on in tests and to aggregate in logs.
type RejectionCode string

const (
	CodeTooLong        RejectionCode = "too_long"
	CodeUnparseable    RejectionCode = "unparseable"
	CodeMultiStatement RejectionCode = "multi_statement"
	CodeNotSelect      RejectionCode = "not_select"
	CodeRecursiveCTE   RejectionCode = "recursive_cte"
	CodeSelectInto     RejectionCode = "select_into"
	CodeLocking        RejectionCode = "locking_clause"
	CodePlaceholder    RejectionCode = "placeholder"
	CodeQualifiedName  RejectionCode = "schema_qualified"
	CodeOnlyModifier   RejectionCode = "only_modifier"
	CodeUnknownTable   RejectionCode = "unknown_table"
	CodeCTEShadow      RejectionCode = "cte_shadows_table"
	CodeUnknownFunc    RejectionCode = "unknown_function"
	CodeDeparse        RejectionCode = "deparse_failed"
)

// RejectionError explains a refusal twice: Detail is for our logs, LLMHint is
// the only part ever fed back to the model. Keeping them separate stops
// internal specifics (table inventory, guard mechanics) from leaking into a
// prompt that a user can influence.
type RejectionError struct {
	Code    RejectionCode
	Detail  string
	LLMHint string
}

func (e *RejectionError) Error() string {
	return fmt.Sprintf("livisql rejected query (%s): %s", e.Code, e.Detail)
}

func reject(code RejectionCode, hint, detail string, args ...any) *RejectionError {
	return &RejectionError{
		Code:    code,
		Detail:  fmt.Sprintf(detail, args...),
		LLMHint: hint,
	}
}
