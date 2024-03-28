#/bin/bash

a=0
while [ $a -lt 2 ]
do
i=0
while [ $i -lt 1 ]
do
  ./godpu evpn create-vrf --name yellow$a --vni `expr $a + 301` --loopback 10.11.17.`expr $i + $a + 10`/32  --vtep 110.1.41.5/32 --addr localhost:50151
  i=`expr $i + 1`
done
sleep 0.5

j=0

while [ $j -lt 1 ]
do
  ./godpu evpn get-vrf --name yellow$a
  j=`expr $j + 1`
done

x=0

while [ $x -lt 1 ]
do
  ./godpu evpn delete-vrf --name yellow$a
  x=`expr $x + 1`
done
a=`expr $a + 1`

done
