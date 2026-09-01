package embedding

// Float32Vector narrows Core's canonical vector representation for stores that
// index in single precision. Conversion belongs at that store boundary rather
// than in providers that cannot know the eventual index format.
func Float32Vector(source []float64) []float32 {
	if source == nil {
		return nil
	}
	output := make([]float32, len(source))
	for i, value := range source {
		output[i] = float32(value)
	}
	return output
}
