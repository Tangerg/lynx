package skills

import "errors"

var (
	// ErrNilSource means NewTool was called without a backing source.
	ErrNilSource = errors.New("skills: source must not be nil")
	// ErrUnknownOp means the op argument was not one of list, load, or
	// load_resource.
	ErrUnknownOp = errors.New("skills: unknown op (want list, load, or load_resource)")
	// ErrNameRequired means a load or load_resource call omitted the skill
	// name.
	ErrNameRequired = errors.New("skills: name is required for load and load_resource")
	// ErrPathRequired means a load_resource call omitted the resource path.
	ErrPathRequired = errors.New("skills: path is required for load_resource")
)
