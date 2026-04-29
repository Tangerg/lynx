// Package bufio provides additional split functions for bufio.Scanner.
//
// [ScanLinesAllFormats] recognizes "\n", "\r", and "\r\n" as line
// terminators, which is convenient for inputs that may mix line
// endings (for example, Server-Sent Events).
package bufio
