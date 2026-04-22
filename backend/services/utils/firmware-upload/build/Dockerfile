FROM node:20-alpine
WORKDIR /app
COPY ../firmware-upload.js .
COPY ../package.json .
COPY ../entrypoint.sh .
RUN apk add --no-cache su-exec && npm install --production && mkdir -p /app/firmwares && chmod +x /app/entrypoint.sh
ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["node", "firmware-upload.js"]
