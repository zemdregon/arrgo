# syntax=docker/dockerfile:1

FROM node:20-alpine AS deps
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1
COPY package.json package-lock.json ./
RUN npm ci

FROM node:20-alpine AS build
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN mkdir -p public
ARG NEXT_PUBLIC_ARRGO_API_URL="http://localhost:8080"
ENV NEXT_PUBLIC_ARRGO_API_URL=${NEXT_PUBLIC_ARRGO_API_URL}
RUN npm run build

FROM node:20-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV HOST=0.0.0.0
ENV PORT=3000
ARG NEXT_PUBLIC_ARRGO_API_URL="http://localhost:8080"
ENV NEXT_PUBLIC_ARRGO_API_URL=${NEXT_PUBLIC_ARRGO_API_URL}
COPY --from=build /app/package.json ./package.json
COPY --from=build /app/package-lock.json ./package-lock.json
RUN npm ci --omit=dev
COPY --from=build /app/.next ./.next
COPY --from=build /app/public ./public
COPY --from=build /app/next.config.mjs ./next.config.mjs
RUN chown -R node:node /app
USER node
EXPOSE 3000
CMD ["npm", "run", "start"]
