FROM node:20-alpine
WORKDIR /app
COPY ../firmware-upload.js .
COPY ../package.json .
COPY ../entrypoint.sh .
RUN npm install --production \
    && mkdir -p /app/firmwares \
    && chown -R node:node /app \
    && chmod +x /app/entrypoint.sh
USER node
ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["node", "firmware-upload.js"]
