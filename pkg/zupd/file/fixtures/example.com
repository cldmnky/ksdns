$ORIGIN example.com.
@                      3600 SOA   ns1.p30.ksdns.net. (
                              zone-admin.dyndns.com.     ; address of responsible party
                              2016072701                 ; serial number
                              3600                       ; refresh period
                              600                        ; retry period
                              604800                     ; expire time
                              1800                     ) ; minimum ttl
                      86400 NS    ns1.p30.ksdns.net.
                      86400 NS    ns2.p30.ksdns.net.
                      86400 NS    ns3.p30.ksdns.net.
                      86400 NS    ns4.p30.ksdns.net.
                       3600 MX    10 mail.example.com.
                       3600 MX    20 vpn.example.com.
                       3600 MX    30 mail.example.com.
                         60 A     204.13.248.106
                       3600 TXT   "v=spf1 includespf.ksdns.net ~all"
mail                  14400 A     204.13.248.106
vpn                      60 A     216.146.45.240
webapp                   60 A     216.146.46.10
webapp                   60 A     216.146.46.11
www                   43200 CNAME example.com.
