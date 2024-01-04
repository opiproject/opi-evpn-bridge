#/bin/bash

<< com
{
  set -e
  while true
  do
    sleep 1
    # Since this is run as a subshell (instead of an external command),
    # the parent pid is $$, not $PPID.
    ip -br l | grep br- | wc -l
 #   kill -9  $(pidof opi-evpn-bridge_12022024)
  done
} &

com

i=0
while [ $i -lt 100 ]
do
  ./godpu evpn create-vrf --name brown$i --vni `expr $i + 100` --loopback 10.121.11.`expr $i + 10`/32  --vtep 110.1.41.51/32 --addr localhost:50151
  i=`expr $i + 1`
# sleep 0.1
done
sleep 40

j=0

while [ $j -lt 100 ]
do
  ./godpu evpn get-vrf --name brown$j
  #sleep 0.5
  j=`expr $j + 1`
done

sleep 30
x=0

while [ $x -lt 100 ]
do
  ./godpu evpn delete-vrf --name brown$x
  #sleep 0.5
  x=`expr $x + 1`
done

