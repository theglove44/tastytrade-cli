# HELP go_gc_duration_seconds A summary of the pause duration of garbage collection cycles.
# TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 0.000118292
go_gc_duration_seconds{quantile="0.25"} 0.000126708
go_gc_duration_seconds{quantile="0.5"} 0.000320583
go_gc_duration_seconds{quantile="0.75"} 0.000424291
go_gc_duration_seconds{quantile="1"} 0.0007275
go_gc_duration_seconds_sum 0.001717374
go_gc_duration_seconds_count 5
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 24
# HELP go_info Information about the Go environment.
# TYPE go_info gauge
go_info{version="go1.25.4"} 1
# HELP go_memstats_alloc_bytes Number of bytes allocated and still in use.
# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes 1.383144e+06
# HELP go_memstats_alloc_bytes_total Total number of bytes allocated, even if freed.
# TYPE go_memstats_alloc_bytes_total counter
go_memstats_alloc_bytes_total 5.731888e+06
# HELP go_memstats_buck_hash_sys_bytes Number of bytes used by the profiling bucket hash table.
# TYPE go_memstats_buck_hash_sys_bytes gauge
go_memstats_buck_hash_sys_bytes 1.444647e+06
# HELP go_memstats_frees_total Total number of frees.
# TYPE go_memstats_frees_total counter
go_memstats_frees_total 11630
# HELP go_memstats_gc_sys_bytes Number of bytes used for garbage collection system metadata.
# TYPE go_memstats_gc_sys_bytes gauge
go_memstats_gc_sys_bytes 2.908944e+06
# HELP go_memstats_heap_alloc_bytes Number of heap bytes allocated and still in use.
# TYPE go_memstats_heap_alloc_bytes gauge
go_memstats_heap_alloc_bytes 1.383144e+06
# HELP go_memstats_heap_idle_bytes Number of heap bytes waiting to be used.
# TYPE go_memstats_heap_idle_bytes gauge
go_memstats_heap_idle_bytes 8.511488e+06
# HELP go_memstats_heap_inuse_bytes Number of heap bytes that are in use.
# TYPE go_memstats_heap_inuse_bytes gauge
go_memstats_heap_inuse_bytes 3.31776e+06
# HELP go_memstats_heap_objects Number of allocated objects.
# TYPE go_memstats_heap_objects gauge
go_memstats_heap_objects 3890
# HELP go_memstats_heap_released_bytes Number of heap bytes released to OS.
# TYPE go_memstats_heap_released_bytes gauge
go_memstats_heap_released_bytes 8.077312e+06
# HELP go_memstats_heap_sys_bytes Number of heap bytes obtained from system.
# TYPE go_memstats_heap_sys_bytes gauge
go_memstats_heap_sys_bytes 1.1829248e+07
# HELP go_memstats_last_gc_time_seconds Number of seconds since 1970 of last garbage collection.
# TYPE go_memstats_last_gc_time_seconds gauge
go_memstats_last_gc_time_seconds 1.773392471538835e+09
# HELP go_memstats_lookups_total Total number of pointer lookups.
# TYPE go_memstats_lookups_total counter
go_memstats_lookups_total 0
# HELP go_memstats_mallocs_total Total number of mallocs.
# TYPE go_memstats_mallocs_total counter
go_memstats_mallocs_total 15520
# HELP go_memstats_mcache_inuse_bytes Number of bytes in use by mcache structures.
# TYPE go_memstats_mcache_inuse_bytes gauge
go_memstats_mcache_inuse_bytes 9664
# HELP go_memstats_mcache_sys_bytes Number of bytes used for mcache structures obtained from system.
# TYPE go_memstats_mcache_sys_bytes gauge
go_memstats_mcache_sys_bytes 15704
# HELP go_memstats_mspan_inuse_bytes Number of bytes in use by mspan structures.
# TYPE go_memstats_mspan_inuse_bytes gauge
go_memstats_mspan_inuse_bytes 116320
# HELP go_memstats_mspan_sys_bytes Number of bytes used for mspan structures obtained from system.
# TYPE go_memstats_mspan_sys_bytes gauge
go_memstats_mspan_sys_bytes 130560
# HELP go_memstats_next_gc_bytes Number of heap bytes when next garbage collection will take place.
# TYPE go_memstats_next_gc_bytes gauge
go_memstats_next_gc_bytes 4.194304e+06
# HELP go_memstats_other_sys_bytes Number of bytes used for other system allocations.
# TYPE go_memstats_other_sys_bytes gauge
go_memstats_other_sys_bytes 1.679225e+06
# HELP go_memstats_stack_inuse_bytes Number of bytes in use by the stack allocator.
# TYPE go_memstats_stack_inuse_bytes gauge
go_memstats_stack_inuse_bytes 753664
# HELP go_memstats_stack_sys_bytes Number of bytes obtained from system for stack allocator.
# TYPE go_memstats_stack_sys_bytes gauge
go_memstats_stack_sys_bytes 753664
# HELP go_memstats_sys_bytes Number of bytes obtained from system.
# TYPE go_memstats_sys_bytes gauge
go_memstats_sys_bytes 1.8761992e+07
# HELP go_threads Number of OS threads created.
# TYPE go_threads gauge
go_threads 12
# HELP promhttp_metric_handler_requests_in_flight Current number of scrapes being served.
# TYPE promhttp_metric_handler_requests_in_flight gauge
promhttp_metric_handler_requests_in_flight 1
# HELP promhttp_metric_handler_requests_total Total number of scrapes by HTTP status code.
# TYPE promhttp_metric_handler_requests_total counter
promhttp_metric_handler_requests_total{code="200"} 3
promhttp_metric_handler_requests_total{code="500"} 0
promhttp_metric_handler_requests_total{code="503"} 0
# HELP tastytrade_circuit_breaker_state 0=normal, 1=tripped
# TYPE tastytrade_circuit_breaker_state gauge
tastytrade_circuit_breaker_state 0
# HELP tastytrade_kill_switch_state 0=normal, 1=halted
# TYPE tastytrade_kill_switch_state gauge
tastytrade_kill_switch_state 0
# HELP tastytrade_last_quote_unix_seconds Unix timestamp of the most recently received quote event (0 = none yet)
# TYPE tastytrade_last_quote_unix_seconds gauge
tastytrade_last_quote_unix_seconds 0
# HELP tastytrade_nlq_dollars Current net liquidating value in USD
# TYPE tastytrade_nlq_dollars gauge
tastytrade_nlq_dollars 0
# HELP tastytrade_open_positions Number of open positions
# TYPE tastytrade_open_positions gauge
tastytrade_open_positions 4
# HELP tastytrade_order_latency_seconds Time from dry-run call to fill event
# TYPE tastytrade_order_latency_seconds histogram
tastytrade_order_latency_seconds_bucket{le="0.1"} 0
tastytrade_order_latency_seconds_bucket{le="0.25"} 0
tastytrade_order_latency_seconds_bucket{le="0.5"} 0
tastytrade_order_latency_seconds_bucket{le="1"} 0
tastytrade_order_latency_seconds_bucket{le="2"} 0
tastytrade_order_latency_seconds_bucket{le="5"} 0
tastytrade_order_latency_seconds_bucket{le="10"} 0
tastytrade_order_latency_seconds_bucket{le="30"} 0
tastytrade_order_latency_seconds_bucket{le="+Inf"} 0
tastytrade_order_latency_seconds_sum 0
tastytrade_order_latency_seconds_count 0
# HELP tastytrade_reconcile_errors_total Reconciliation passes that failed due to a REST error. Non-zero values indicate connectivity or auth problems with the Positions endpoint.
# TYPE tastytrade_reconcile_errors_total counter
tastytrade_reconcile_errors_total 0
# HELP tastytrade_reconcile_positions_corrected_total MarkBook entries patched by the reconciler (new positions added + zero-cost-basis entries corrected).
# TYPE tastytrade_reconcile_positions_corrected_total counter
tastytrade_reconcile_positions_corrected_total 0
# HELP tastytrade_reconcile_runs_total Total reconciliation passes attempted (success + failure).
# TYPE tastytrade_reconcile_runs_total counter
tastytrade_reconcile_runs_total 10
# HELP tastytrade_request_duration_seconds HTTP request duration per family and method
# TYPE tastytrade_request_duration_seconds histogram
tastytrade_request_duration_seconds_bucket{family="market_data",method="GET",le="0.005"} 0
tastytrade_request_duration_seconds_bucket{family="market_data",method="GET",le="0.01"} 0
tastytrade_request_duration_seconds_bucket{family="market_data",method="GET",le="0.025"} 0
tastytrade_request_duration_seconds_bucket{family="market_data",method="GET",le="0.05"} 0
tastytrade_request_duration_seconds_bucket{family="market_data",method="GET",le="0.1"} 0
tastytrade_request_duration_seconds_bucket{family="market_data",method="GET",le="0.25"} 1
tastytrade_request_duration_seconds_bucket{family="market_data",method="GET",le="0.5"} 1
tastytrade_request_duration_seconds_bucket{family="market_data",method="GET",le="1"} 1
tastytrade_request_duration_seconds_bucket{family="market_data",method="GET",le="2.5"} 1
tastytrade_request_duration_seconds_bucket{family="market_data",method="GET",le="5"} 1
tastytrade_request_duration_seconds_bucket{family="market_data",method="GET",le="10"} 1
tastytrade_request_duration_seconds_bucket{family="market_data",method="GET",le="+Inf"} 1
tastytrade_request_duration_seconds_sum{family="market_data",method="GET"} 0.118406083
tastytrade_request_duration_seconds_count{family="market_data",method="GET"} 1
tastytrade_request_duration_seconds_bucket{family="read",method="GET",le="0.005"} 0
tastytrade_request_duration_seconds_bucket{family="read",method="GET",le="0.01"} 0
tastytrade_request_duration_seconds_bucket{family="read",method="GET",le="0.025"} 0
tastytrade_request_duration_seconds_bucket{family="read",method="GET",le="0.05"} 0
tastytrade_request_duration_seconds_bucket{family="read",method="GET",le="0.1"} 0
tastytrade_request_duration_seconds_bucket{family="read",method="GET",le="0.25"} 11
tastytrade_request_duration_seconds_bucket{family="read",method="GET",le="0.5"} 11
tastytrade_request_duration_seconds_bucket{family="read",method="GET",le="1"} 11
tastytrade_request_duration_seconds_bucket{family="read",method="GET",le="2.5"} 11
tastytrade_request_duration_seconds_bucket{family="read",method="GET",le="5"} 11
tastytrade_request_duration_seconds_bucket{family="read",method="GET",le="10"} 11
tastytrade_request_duration_seconds_bucket{family="read",method="GET",le="+Inf"} 11
tastytrade_request_duration_seconds_sum{family="read",method="GET"} 1.27222225
tastytrade_request_duration_seconds_count{family="read",method="GET"} 11
# HELP tastytrade_streamer_uptime_seconds Seconds since last successful streamer connection
# TYPE tastytrade_streamer_uptime_seconds gauge
tastytrade_streamer_uptime_seconds{streamer="account"} 600.002515417
tastytrade_streamer_uptime_seconds{streamer="market"} 600.0024055
# HELP tastytrade_token_refresh_total Token refresh attempts
# TYPE tastytrade_token_refresh_total counter
tastytrade_token_refresh_total{outcome="missing_refresh_token"} 1
# HELP tastytrade_tracked_symbols Number of symbols currently subscribed on the market streamer
# TYPE tastytrade_tracked_symbols gauge
tastytrade_tracked_symbols 4