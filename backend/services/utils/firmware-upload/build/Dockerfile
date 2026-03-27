FROM node:20-alpine
WORKDIR /app
COPY ../firmware-upload.js .
COPY ../package.json .
RUN npm install --production && mkdir -p /app/firmwares && chown -R node:node /app
USER node
CMD ["node", "firmware-upload.js"]
