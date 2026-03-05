FROM node:20-alpine
WORKDIR /app
COPY ../firmware-upload.js .
COPY ../package.json .
RUN npm install --production
CMD ["node", "firmware-upload.js"]
