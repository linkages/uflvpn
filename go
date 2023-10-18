#!/bin/bash
# requires: tmux openconnect systemd(resolvectl) iproute2(ip) coreutils(dirname) sed awk

# load up the yaml parser
source ./lib/parse_yaml
source ./lib/functions

while getopts "c:" opt; do
    case ${opt} in
	c)
	    configFile=${OPTARG}
	    ;;
    esac
done

if [ -v configFile ]; then
    config=${configFile}
else
    config="./config.yaml"
fi

if [ -f ${config} ]; then
    eval $(parse_yaml ${config})
else
    echo "Failed to find configuration file: [${config}]"
    exit 2
fi

check_args

# Setup logging
if [ ! -d ${logs} ]; then
    mkdir -p ${logs}
fi

NETWORKS=""
for network in ${networks_}; do
    NETWORKS=$(echo ${NETWORKS} $(eval echo \${${network}}))
done

DNS_SERVERS=""
if [ check_dns ]; then
    log "Going to use customer dns servers"
    for dns_server in ${dns_servers_}; do
	DNS_SERVERS=$(echo ${DNS_SERVERS} $(eval echo \${${dns_server}}))
    done
fi

DOMAINS=""
if [ check_domains ]; then
    log "Going to use custom domain search"
    for domain in ${domains_}; do
	DOMAINS=$(echo ${DOMAINS} $(eval echo \${${domain}}))
    done
fi

MYGATEWAY=$(ip route list | grep default | awk '{print $3}')
WINDOWNAME=${tun_ifac//./-}

RUN_USER=${USER}
SOK_NAME=${user}.${host}
STY_NAME=${PPID}.${SOK_NAME}

function add_dns
{
    resolvectl dns ${tun_ifac} ${DNS_SERVERS}
    resolvectl domain ${tun_ifac} ${DOMAINS}
}

function remove_dns
{
    resolvectl dns ${tun_ifac} ""
    resolvectl domain ${tun_ifac} ""
}

# connects to VPN, does not return. 
function vpn_connect
{
    if [ -v certfingerprint ]; then
	exec openconnect \
	     --interface=${tun_ifac} \
	     --setuid=${RUN_USER} \
	     --user="${user}@ufl.edu/${tunnelname}" \
	     --script="${0} -c ${config}" \
	     --servercert="${certfingerprint}" \
	     ${host}
    else
	exec openconnect \
	     --interface=${tun_ifac} \
	     --setuid=${RUN_USER} \
	     --user="${user}@ufl.edu/${tunnelname}" \
	     --script="${0} -c ${config}" \
	     ${host}
    fi
}

# adds routes, called indirectly from openconnect --script
function add_routes
{
    log "Adding requested routes";
    for net in ${NETWORKS}; do
	log "ip route add ${net} dev ${tun_ifac} scope link"
	ip route add ${net} dev ${tun_ifac} scope link
    done

    MYIP=$(ip addr list ${nic} | grep -w inet | awk '{print $2}' | awk -F / '{print $1}')
    log "Adding route to ${VPNGATEWAY} via ${MYIP}"
    log "ip route add ${VPNGATEWAY} via ${MYGATEWAY} dev ${nic} source ${MYIP}"
    ip route add ${VPNGATEWAY} via ${MYGATEWAY} dev ${nic}
    ip route list
}

function remove_routes
{
    MYIP=$(ip addr list ${nic} | grep -w inet | awk '{print $2}' | awk -F / '{print $1}')
    log "Removing route to ${VPNGATEWAY} via ${MYIP}"
    log "ip route del ${VPNGATEWAY} via ${MYGATEWAY} dev ${nic} source ${MYIP}"
    ip route del ${VPNGATEWAY} via ${MYGATEWAY} dev ${nic}
    ip route list
}

# check if openconnect is making this call
if [ "${TUNDEV}" == "${tun_ifac}" ]; then
    # if so, then if this "connect" then we need to bring up the interface and setup routes
    if [ "${reason}" == "connect" ]; then
	log "Setting IP addr on interface [${tun_ifac}] to ${INTERNAL_IP4_ADDRESS}/${INTERNAL_IP4_NETMASKLEN}"
	ip addr add ${INTERNAL_IP4_ADDRESS}/${INTERNAL_IP4_NETMASKLEN} dev ${tun_ifac}
	ip link set dev ${tun_ifac} up
	log "Calling add routes. reason=${reason}";
	add_routes
	add_dns
	exit 0
    fi

    if [ "${reason}" == "disconnect" ]; then
	log "Calling remove routes. reason=${reason}";
	remove_routes
	remove_dns
	exit 0
    fi
    
    log "Exiting due to unknown reason. reason=${reason}";
    exit 0
fi

# check if VPN has already been created
ip link show dev ${tun_ifac} >/dev/null 2>&1

if [ $? -eq 0 ]; then
    log "VPN is already running."
    exit 2
fi

vpn_connect
