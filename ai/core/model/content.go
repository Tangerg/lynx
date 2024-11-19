package model

// Content defines an interface for types that provide content along with
// associated metadata. It is useful for handling entities where textual or
// other forms of content are accompanied by additional descriptive information.
//
// Methods:
//
// Content:
//
//	Content() string
//	Retrieves the content as a string.
//	Returns:
//	- string: The content represented as a string.
//
// Metadata:
//
//	Metadata() map[string]any
//	Retrieves metadata associated with the content. The metadata is represented
//	as a map where the keys are strings and the values can be of any type.
//	Returns:
//	- map[string]any: A map containing metadata key-value pairs.
//
// Example Implementation:
//
//	type Article struct {
//	    text     string
//	    metadata map[string]any
//	}
//
//	func (a *Article) Content() string {
//	    return a.text
//	}
//
//	func (a *Article) Metadata() map[string]any {
//	    return a.metadata
//	}
//
// Example Usage:
//
//	article := &Article{
//	    text: "This is the article content.",
//	    metadata: map[string]any{
//	        "author": "John Doe",
//	        "length": 1200,
//	    },
//	}
//	fmt.Println(article.Content())             // Output: This is the article content.
//	fmt.Println(article.Metadata()["author"]) // Output: John Doe
type Content interface {
	// Content returns the content as a string.
	Content() string

	// Metadata returns a map containing metadata information.
	// The map keys are strings, and the values can be of any type.
	Metadata() map[string]any
}
