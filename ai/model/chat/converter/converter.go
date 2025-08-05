package converter

// StructuredConverter defines an interface for converting unstructured LLM (Large Language Model)
// output into structured data of type T. This interface addresses the common challenge where LLMs
// generate natural language responses that need to be transformed into specific data structures.
//
// The interface follows a two-phase approach:
//  1. GetFormat() provides formatting instructions to guide the LLM's output generation
//  2. Convert() transforms the raw LLM response into the desired structured format
//
// Type parameter T represents the target data structure that the raw LLM output will be converted to.
// T can be any type including structs, slices, maps, or primitive types.
type StructuredConverter[T any] interface {
	// GetFormat returns a string containing detailed formatting instructions that should be
	// included in the LLM prompt to guide the model's output generation. These instructions
	// describe the expected structure, format, and constraints for the response.
	//
	// The returned string is typically appended to the user's prompt to inform the LLM
	// about the desired output format. This may include JSON schema requirements,
	// list formatting rules, field specifications, or other structural constraints.
	//
	// Returns a string containing formatting instructions for the LLM.
	GetFormat() string

	// Convert transforms the raw, unstructured output from an LLM into a structured
	// response of type T. This method handles the parsing, validation, and type conversion
	// necessary to extract meaningful data from natural language responses.
	//
	// The implementation should be robust enough to handle variations in LLM output format,
	// including extra explanatory text, slight formatting deviations, or minor inconsistencies
	// that commonly occur in language model responses.
	//
	// Parameters:
	//   - raw: The unprocessed string response from the LLM, which may contain the target
	//     data mixed with additional natural language text, formatting, or explanations
	//
	// Returns:
	//   - T: The successfully parsed and converted structured data
	//   - error: An error if the conversion fails due to parsing issues, validation failures,
	//     or if the raw input doesn't contain the expected data structure
	//
	// Common error conditions include invalid format, missing required fields, type conversion
	// failures, or malformed structured data within the raw response.
	Convert(raw string) (T, error)
}
