// Package ident validates the SQL-style identifiers that vector
// stores interpolate into DDL and search SQL. Two patterns cover the
// vector-store providers in this module: the strict SQL-unquoted shape
// (most backends) and a hyphen-accepting variant (Couchbase, Vespa).
package ident
