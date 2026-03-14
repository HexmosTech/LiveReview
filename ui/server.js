const express = require('express');
const cookieParser = require('cookie-parser');
const csrf = require('csurf');
const { createProxyMiddleware } = require('http-proxy-middleware');
const path = require('path');

const app = express();
const PORT = process.env.PORT || 3000;
const csrfProtection = csrf({
  cookie: {
    httpOnly: true,
    sameSite: 'lax',
    secure: process.env.NODE_ENV === 'production',
  },
});

app.use(cookieParser());

app.get('/csrf-token', csrfProtection, (req, res) => {
  res.json({ csrfToken: req.csrfToken() });
});

const requireCsrfForMutations = (req, res, next) => {
  if (req.method === 'GET' || req.method === 'HEAD' || req.method === 'OPTIONS') {
    return next();
  }
  return csrfProtection(req, res, next);
};

// Serve static files from the React app
app.use(express.static(path.join(__dirname, 'build')));

// Proxy API requests to the backend server
app.use('/api', requireCsrfForMutations, createProxyMiddleware({
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

app.use((err, req, res, next) => {
  if (err && err.code === 'EBADCSRFTOKEN') {
    res.status(403).json({ error: 'Invalid CSRF token' });
    return;
  }
  next(err);
});

// All remaining requests return the React app, so it can handle routing
app.get('*', (req, res) => {
  res.sendFile(path.join(__dirname, 'build', 'index.html'));
});

app.listen(PORT, () => {
  console.log(`Frontend server listening on port ${PORT}`);
  console.log(`Proxying API requests to backend at http://localhost:8888`);
});
