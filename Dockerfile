FROM node:26-alpine AS build

WORKDIR /builder

COPY package.json package-lock.json ./

RUN npm ci

COPY . .

RUN yarn build

FROM node:26-alpine

ENV NODE_ENV=production
ENV PORT=3000
ENV CACHE_DIR=/cache

RUN mkdir -p $CACHE_DIR

WORKDIR /app

COPY package.json package-lock.json ./

RUN npm ci

COPY --from=build /builder/dist ./dist

EXPOSE ${PORT}

CMD [ "node", "dist/index.js" ]
