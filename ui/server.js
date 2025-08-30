const express = require('express');
const { createProxyMiddleware } = require('http-proxy-middleware');
const path = require('path');

const app = express();
const PORT = process.env.PORT || 3000;

// Serve static files from the React app
app.use(express.static(path.join(__dirname, 'build')));

// Proxy API requests to the backend server
app.use('/api', createProxyMiddleware({
  target: 'http://localhost:8888',
  changeOrigin: true,
  pathRewrite: {
    '^/api': '/api', // Rewrite path if needed
  },
  onProxyReq: (proxyReq, req, res) => {
    // Log the request for debugging
    console.log(`Proxying request to: ${req.method} ${proxyReq.path}`);
  },
}));

// All remaining requests return the React app, so it can handle routing
app.get('*', (req, res) => {
  res.sendFile(path.join(__dirname, 'build', 'index.html'));
});

app.listen(PORT, () => {
  console.log(`Frontend server listening on port ${PORT}`);
  console.log(`Proxying API requests to backend at http://localhost:8888`);
});
