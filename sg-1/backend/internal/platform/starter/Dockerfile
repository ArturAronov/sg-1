FROM node:24-alpine
WORKDIR /app
COPY server.js ./
USER node
ENV PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=2s --timeout=2s --start-period=2s --retries=10 CMD node -e "fetch('http://127.0.0.1:8080/healthz').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"
CMD ["node", "server.js"]
