const http = require("node:http");
http.createServer((req, res) => {
  if (req.url === "/healthz") { res.writeHead(200, { "content-type": "application/json" }); res.end('{"status":"ok"}'); return; }
  res.writeHead(200, { "content-type": "text/html; charset=utf-8" });
  res.end(`<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>Your project is running</title></head><body style="margin:0;min-height:100vh;display:grid;place-items:center;background:#f5f8f0;font-family:system-ui;color:#234d3d"><main style="padding:32px;text-align:center;max-width:480px"><div style="font-size:50px">✳</div><h1>Your project is running.</h1><p style="line-height:1.8;color:#68795b">This is a real, isolated application container. Describe your tool in the SG-1 builder to replace this starter with your application.</p></main></body></html>`);
}).listen(Number(process.env.PORT || 8080), "0.0.0.0");
