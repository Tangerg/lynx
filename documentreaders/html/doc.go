// Package html implements a [document.Reader] over HTML payloads using
// github.com/PuerkitoBio/goquery.
//
// The reader extracts visible text from an HTML document. Two modes:
//
//   - Whole-document mode (default): a single [*document.Document]
//     containing the body text with title / description / canonical URL
//     stamped as metadata.
//   - Selector mode (opt in via [WithSelector]): emits one document per
//     element matched by the CSS selector — useful for scraping blog
//     post lists, search results, etc.
//
// Example:
//
//	r, _ := html.NewReader(strings.NewReader(htmlSrc))
//	docs, _ := r.Read(ctx)
//
//	r, _ := html.NewReader(strings.NewReader(htmlSrc),
//	    html.WithSelector("article"))
//	docs, _ := r.Read(ctx) // one doc per <article>
package html
