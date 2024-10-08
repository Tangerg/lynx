package id

type Generator interface {
	GenerateId(obj ...any) string
}
