FROM node:26-alpine AS build

WORKDIR /builder

COPY package.json package-lock.json ./

RUN npm ci

COPY . .

RUN npm run build

FROM node:26-alpine

ENV NODE_ENV=production
ENV PORT=3000

WORKDIR /app

COPY package.json package-lock.json ./

RUN npm ci

COPY --from=build /builder/dist ./dist

EXPOSE ${PORT}

CMD [ "node", "dist/index.js" ]
