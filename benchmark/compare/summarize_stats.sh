#!/usr/bin/env bash
set -euo pipefail

# Summarize CPU/RSS CSV to a single line
# Usage: ./summarize_stats.sh <csv_file>

CSV="${1:?csv file required}"

# Columns: timestamp,cpu_percent,rss_kb
# Output: cpu_avg,cpu_max,rss_avg_kb,rss_max_kb,rss_avg_mb,rss_max_mb
awk -F',' 'NR>1 {cpu=$2; rss=$3; cpu_sum+=cpu; rss_sum+=rss; if(cpu>cpu_max) cpu_max=cpu; if(rss>rss_max) rss_max=rss; n++}
END {
  if(n==0){print "NA,NA,NA,NA,NA,NA"; exit 0}
  rss_avg_kb = rss_sum/n
  rss_max_kb = rss_max
  rss_avg_mb = rss_avg_kb/1024
  rss_max_mb = rss_max_kb/1024
  printf "%.2f,%.2f,%.0f,%.0f,%.2f,%.2f\n", cpu_sum/n, cpu_max, rss_avg_kb, rss_max_kb, rss_avg_mb, rss_max_mb
}' "$CSV"
