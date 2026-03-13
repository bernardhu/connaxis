#!/usr/bin/env bash
if [[ -z "${BASH_VERSION:-}" ]]; then
  exec /usr/bin/env bash "$0" "$@"
fi
set -euo pipefail

OUT="${1:-results/env.json}"

mkdir -p "$(dirname "$OUT")"

env_date=$(date -Iseconds)

os_kernel=$(uname -a)

if [[ "$(uname -s)" == "Darwin" ]]; then
  # macOS friendly sysctl queries
  cpu_model=$(sysctl -n machdep.cpu.brand_string 2>/dev/null || echo "unknown")
  cores=$(sysctl -n hw.ncpu 2>/dev/null || echo "unknown")
  mem_bytes=$(sysctl -n hw.memsize 2>/dev/null || echo "unknown")
  sysctl_ostype=$(sysctl -n kern.ostype 2>/dev/null || echo "unknown")
  sysctl_osrelease=$(sysctl -n kern.osrelease 2>/dev/null || echo "unknown")
  sysctl_version=$(sysctl -n kern.version 2>/dev/null || echo "unknown")
  sysctl_pagesize=$(sysctl -n hw.pagesize 2>/dev/null || echo "unknown")
sysctl_cacheline=$(sysctl -n hw.cachelinesize 2>/dev/null || echo "unknown")
sysctl_l1d=$(sysctl -n hw.l1dcachesize 2>/dev/null || echo "unknown")
sysctl_l2=$(sysctl -n hw.l2cachesize 2>/dev/null || echo "unknown")
  sysctl_l3=$(sysctl -n hw.l3cachesize 2>/dev/null || echo "unknown")
  net_sendbuf=$(sysctl -n net.inet.tcp.sendspace 2>/dev/null || echo "unknown")
  net_recvbuf=$(sysctl -n net.inet.tcp.recvspace 2>/dev/null || echo "unknown")
else
  # Linux/Ubuntu
  cpu_model=$(lscpu 2>/dev/null | awk -F: '/Model name/ {gsub(/^[ \t]+/,"",$2); print $2; exit}' || echo "unknown")
  cores=$(nproc 2>/dev/null || echo "unknown")
  mem_bytes=$(awk '/MemTotal/ {printf "%d", $2*1024}' /proc/meminfo 2>/dev/null || echo "unknown")
  sysctl_ostype=$(sysctl -n kernel.ostype 2>/dev/null || echo "unknown")
  sysctl_osrelease=$(sysctl -n kernel.osrelease 2>/dev/null || echo "unknown")
  sysctl_version=$(sysctl -n kernel.version 2>/dev/null || echo "unknown")
  sysctl_pagesize=$(getconf PAGESIZE 2>/dev/null || echo "unknown")
  sysctl_cacheline=$(getconf LEVEL1_DCACHE_LINESIZE 2>/dev/null || echo "unknown")
  sysctl_l1d=$(lscpu 2>/dev/null | awk -F: '/L1d cache/ {gsub(/^[ \t]+/,"",$2); print $2; exit}' || echo "unknown")
  sysctl_l2=$(lscpu 2>/dev/null | awk -F: '/L2 cache/ {gsub(/^[ \t]+/,"",$2); print $2; exit}' || echo "unknown")
  sysctl_l3=$(lscpu 2>/dev/null | awk -F: '/L3 cache/ {gsub(/^[ \t]+/,"",$2); print $2; exit}' || echo "unknown")
  net_sendbuf=$(sysctl -n net.core.wmem_max 2>/dev/null || echo "unknown")
  net_recvbuf=$(sysctl -n net.core.rmem_max 2>/dev/null || echo "unknown")
fi

# Network sysctl (best effort)
net_somaxconn=$(sysctl -n net.core.somaxconn 2>/dev/null || echo "unknown")
net_backlog=$(sysctl -n net.core.netdev_max_backlog 2>/dev/null || echo "unknown")
net_syn_backlog=$(sysctl -n net.ipv4.tcp_max_syn_backlog 2>/dev/null || echo "unknown")
net_fin_timeout=$(sysctl -n net.ipv4.tcp_fin_timeout 2>/dev/null || echo "unknown")
net_tw_reuse=$(sysctl -n net.ipv4.tcp_tw_reuse 2>/dev/null || echo "unknown")
net_timestamps=$(sysctl -n net.ipv4.tcp_timestamps 2>/dev/null || echo "unknown")
net_keepalive_time=$(sysctl -n net.ipv4.tcp_keepalive_time 2>/dev/null || echo "unknown")
net_keepalive_intvl=$(sysctl -n net.ipv4.tcp_keepalive_intvl 2>/dev/null || echo "unknown")
net_keepalive_probes=$(sysctl -n net.ipv4.tcp_keepalive_probes 2>/dev/null || echo "unknown")
go_ver=$(go version 2>/dev/null || echo "unknown")
gomax=$(go env GOMAXPROCS 2>/dev/null || echo "unknown")

cat > "$OUT" <<JSON
{
  "date": "$env_date",
  "cpu_model": "$cpu_model",
  "cores": "$cores",
  "mem_bytes": "$mem_bytes",
  "os_kernel": "$os_kernel",
  "sysctl_ostype": "$sysctl_ostype",
  "sysctl_osrelease": "$sysctl_osrelease",
  "sysctl_version": "$sysctl_version",
  "sysctl_pagesize": "$sysctl_pagesize",
  "sysctl_cachelinesize": "$sysctl_cacheline",
  "sysctl_l1dcachesize": "$sysctl_l1d",
  "sysctl_l2cachesize": "$sysctl_l2",
  "sysctl_l3cachesize": "$sysctl_l3",
  "net_sendbuf": "$net_sendbuf",
  "net_recvbuf": "$net_recvbuf",
  "net_core_somaxconn": "$net_somaxconn",
  "net_core_netdev_max_backlog": "$net_backlog",
  "net_ipv4_tcp_max_syn_backlog": "$net_syn_backlog",
  "net_ipv4_tcp_fin_timeout": "$net_fin_timeout",
  "net_ipv4_tcp_tw_reuse": "$net_tw_reuse",
  "net_ipv4_tcp_timestamps": "$net_timestamps",
  "net_ipv4_tcp_keepalive_time": "$net_keepalive_time",
  "net_ipv4_tcp_keepalive_intvl": "$net_keepalive_intvl",
  "net_ipv4_tcp_keepalive_probes": "$net_keepalive_probes",
  "go_version": "$go_ver",
  "gomaxprocs": "$gomax"
}
JSON

echo "Wrote $OUT"
