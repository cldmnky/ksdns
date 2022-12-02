; https://gist.github.com/magnetikonline/70625d14aabe25a227e3
$ORIGIN myzone.com.
$TTL 1D
@ IN SOA ns1.myzone.com. hostmaster.myzone.com. (
	2015010100  ; serial
	21600       ; refresh
	3600        ; retry
	604800      ; expire
	86400 )     ; minimum TTL
;
@		IN  NS  ns1
ns1		IN  A   123.16.123.1	; glue record
ns1sub	IN	A	123.16.123.10	; glue record
;
;
$ORIGIN sub.myzone.com.
$TTL 1D
@		IN  NS  ns1sub.myzone.com.