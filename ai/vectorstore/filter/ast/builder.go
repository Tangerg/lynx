package ast

type Builder struct {
}

func (b *Builder) EQ()    {}
func (b *Builder) NE()    {}
func (b *Builder) LT()    {}
func (b *Builder) LE()    {}
func (b *Builder) GT()    {}
func (b *Builder) GE()    {}
func (b *Builder) AND()   {}
func (b *Builder) OR()    {}
func (b *Builder) NOT()   {}
func (b *Builder) IN()    {}
func (b *Builder) LIKE()  {}
func (b *Builder) PARAN() {}

func NewBuilder() *Builder { return &Builder{} }
