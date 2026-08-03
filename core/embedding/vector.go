package embedding

// Float32Vector converts a provider-neutral float64 vector to the float32
// representation used by storage SDKs. A nil input remains nil.
func Float32Vector(source []float64) []float32 {
	if source == nil {
		return nil
	}
	result := make([]float32, len(source))
	for i, value := range source {
		result[i] = float32(value)
	}
	return result
}
