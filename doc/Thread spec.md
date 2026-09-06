Here is an outline of a network proxy. Sketch out the main.go and proxy.go files 
focusing particularly on the program logic of when and where os.Signal, context.context,
sync.WaitGroup, and errorgroup are used with respect to the creation and shutdown of the following 
threads. Keep the code idiomatic and simple. Assume use of go 1.26+. Do not bother
with configuration (assume main.go creates a "cfg" configuration structure), logging 
(use log.slog where logging is important to understanding the program logic), 
or command line flags (assume that "cfg" has everything needed).


thread #0: main thread (in "main.go")
-- contains an ordinary function call to proxy.Start(...)
-- waits for graceful shutdown

thread #1: accept thread (in "proxy.go")
-- accept go routine started from proxy.Start(...)
-- loops on accepting local connection requests
-- for each accept starts a connection handling thread
-- accept thread lifetime is that of the main program

thread #2: connection handling thread (in "proxy.go")
-- composes a bidirectional proxy connection
-- on remote connection dial error, attempts to wake remote server (stub the wake-up function call)
-- on remote connection dial success, starts 2 unidirectional data transfer threads
-- waits for shutdown of the data transfer threads using errorgroup.Wait()
-- connection handling thread lifetime is that of its connection

thread #3
-- performs local <- remote data transfer copy
-- terminates on connection close, context done, or transfer error
-- thread lifetime is that of its unidirectional connection

thread #4
-- performs remote <- local data transfer copy
-- terminates on connection close, context done, or transfer error
-- thread lifetime is that of its unidirectional connection
