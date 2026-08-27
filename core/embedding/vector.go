package embedding

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
