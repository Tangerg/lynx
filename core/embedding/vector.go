package embedding

// Float32Vector converts a provider-neutral float64 vector to the float32
// representation used by storage SDKs. A nil input remains nil.
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
