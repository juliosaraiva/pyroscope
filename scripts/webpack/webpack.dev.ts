import { merge } from 'webpack-merge';
import { WebpackPluginServe } from 'webpack-plugin-serve';
import path from 'path';
import HtmlWebpackPlugin from 'html-webpack-plugin';
import request from 'sync-request';
import fs from 'fs';
import route from 'koa-route';
import common from './webpack.common';

module.exports = merge(common, {
  devtool: 'eval-source-map',
  mode: 'development',
  entry: {
    serve: 'webpack-plugin-serve/client',
  },
  plugins: [
    // create a server on port 4041 with live reload
    // it will serve all static assets com webapp/public/assets
    // and for the endpoints it will redirect to the go server (on port 4040)
    new WebpackPluginServe({
      port: 4041,
      static: path.resolve(__dirname, '../../webapp/public'),
      liveReload: true,
      waitForBuild: true,
      middleware: (app, builtins) => {
        // TODO
        // this sucks, maybe update endpoints to prefix with /api?
        app.use(builtins.proxy('/render', { target: 'http://localhost:4040' }));
        app.use(
          builtins.proxy('/render-diff', { target: 'http://localhost:4040' })
        );
        app.use(builtins.proxy('/labels', { target: 'http://localhost:4040' }));
        app.use(
          builtins.proxy('/labels-diff', { target: 'http://localhost:4040' })
        );
        app.use(
          builtins.proxy('/label-values', { target: 'http://localhost:4040' })
        );

        // New Endpoints are implemented under /api
        app.use(builtins.proxy('/api', { target: 'http://localhost:4040' }));

        // serve index for all pages
        // that are not static (.css, .js) nor live reload (/wps)
        // TODO: simplify this
        app.use(
          route.get(/^(.(?!(\.js|\.css|\.svg|wps)$))+$/, (ctx) => {
            ctx.body = fs.readFileSync(
              path.resolve(__dirname, '../../webapp/public/assets/index.html'),
              {
                encoding: 'utf-8',
              }
            );
          })
        );
      },
    }),

    // serve index.html from the go server
    // and additionally inject anything else required (eg livereload ws)
    new HtmlWebpackPlugin({
      publicPath: '/assets',
      templateContent: () => {
        let res;

        // TODO: accept this to be overwritten?
        // that's useful for when running on a different port (when you are running multiple pyroscope versions locally)
        // or when running on ipv6
        const goServerAddr = 'http://localhost:4040';

        try {
          process.stderr.write(`Trying to access go server on ${goServerAddr}`);

          // makes a request against the go server to retrieve its index.html
          // it assumes the server will either not respond or respond with 2xx
          // (ie it doesn't handle != 2xx status codes)
          // https://www.npmjs.com/package/sync-request
          res = request('GET', goServerAddr, {
            timeout: 1000,
            maxRetries: 30,
            retryDelay: 100,
            retry: true,
          });
        } catch (e) {
          throw new Error(
            `Could not find pyroscope instance running on ${goServerAddr}. Make sure you have pyroscope server running on port :4040`
          );
        }

        process.stderr.write('Live reload server is up');

        return res.getBody('utf8');
      },
    }),
  ],
  // TODO deal with these types
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
} as any);
