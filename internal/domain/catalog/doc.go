// Package catalog models what a target EU cloud provider (OVHcloud, Scaleway,
// UpCloud) offers: its offerings, the taxonomy categories and capabilities each
// one covers, the EU regions it is available in, and the dated, sourced snapshot
// the data was taken from.
//
// A Catalog is validated when it is built: a target, a snapshot source and
// date, uniquely named offerings and known categories and capabilities. Its
// offerings are kept in name order so everything derived from it is
// deterministic.
//
// Domain layer: standard library only, no I/O.
package catalog
