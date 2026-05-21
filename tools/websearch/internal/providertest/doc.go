// Package providertest is a tiny shared harness for websearch
// provider tests. Each provider's _test.go calls [Run] with its
// constructor and env-var name; all the boilerplate (skip-when-key-
// missing, smoke-search, JSON dump) lives here once.
package providertest
