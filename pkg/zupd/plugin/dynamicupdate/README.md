# dynamicupdate

## Name

dynamicupdate - enables dynamic updates according to RFC 2136

## Description

The dynamic update plugin mimics `file` plugin to allow RFC 2136 dynamic updates to zones, but the files are read from CRD¨s in the the cluster.

Slightly patched coredns

It stores dynamic update transactions to an index on disk and replays these onto the zones served if restarted

Use with external-dns

Leader election (only one may run)

AXFR Transfers

Store state in CRD's (status)

## Syntax

---
dynamicupdate [NAMESPACES...]
---

---corefile
. {
    dynamicupdate sub.example.net sub.example.net
}
---