$ORIGIN sub.myzone.com.
$TTL 1D
@ IN SOA ns1.sub.myzone.com. hostmaster.sub.myzone.com. (
	2015010100  ; serial
	21600       ; refresh
	3600        ; retry
	604800      ; expire
	86400 )     ; minimum TTL
;
@		IN  NS  ns1
ns1		IN  A   123.16.123.1