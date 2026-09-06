Need a PlantUML sequence diagram to describe the control flow within a Wake on Lan proxy server. Needed to plan the placement, use, and handling of context.Context, *sync.WaitGroup, and net.Conn.Close()

A: func (s *Server) Start() creates a listener and then runs a go routine #1 to accept local connections (intension is to proxy TCP request packets from the local system to a remote server and receive back TCP response packets)

B: for each accepted connection, go routine #1 calls s.handleConnection(ctx, clientConn)

C: s.handleConnection(ctx, clientConn) calls s.makeConnection(ctx, target) to obtain a valid server connection and then calls s.proxy(localConn, serverConn) to implement the proxy data transfers

D: s.makeConnection(ctx, target) implements a loop of s.cfg.WOLMaxRetries with each iteration (i) dialing (with timeout interval of s.cfg.ProxyDialTimeout) to get a serverConn; (ii) on error, sending a WOL magic packet using s.wol.Send(); (iii) on dial success returning serverConn; (iv) on expiration of the max retries, do cleanup and return error.

E: s.proxy(localConn, serverConn) implements two go routines for respectively handling local <- remote and remote <- local packet transfers. The remote <- local flow is interesting as it must recognize when the server might sleep waiting for use and send WOL packet(s) to wake it up.
