const path = require('path');
const HtmlWebpackPlugin = require("html-webpack-plugin");
const MiniCssExtractPlugin = require("mini-css-extract-plugin");
const TerserPlugin = require('terser-webpack-plugin');
const CssMinimizerPlugin = require('css-minimizer-webpack-plugin');
const CopyPlugin = require('copy-webpack-plugin');
const ForkTsCheckerWebpackPlugin = require('fork-ts-checker-webpack-plugin');
const BundleAnalyzerPlugin = require('webpack-bundle-analyzer').BundleAnalyzerPlugin;
const WebpackObfuscator = require('webpack-obfuscator');
const webpack = require('webpack');
const fs = require('fs');
const metaConfig = require('./meta.config.js');

module.exports =  (env, options)=> {

    const devMode = options.mode === 'development' ? true : false;

    process.env.NODE_ENV = options.mode;

    // Load root .env so UI can inherit server configuration (single source of truth)
    const rootEnvPath = path.resolve(__dirname, '..', '.env.prod');
    if (fs.existsSync(rootEnvPath)) {
        const dotenv = require('dotenv');
        dotenv.config({ path: rootEnvPath });
    }

    return {
        mode: options.mode,
        entry: path.resolve(__dirname, './src/index.tsx'),
        output: {
            path: path.resolve(__dirname, './dist'),
            filename: '[name].[contenthash].js',
            chunkFilename: '[name].[contenthash].js',
            clean: true
        },
        devtool: devMode ? 'source-map' : false,
        devServer: {
            port: 8081,
            hot: true,
            historyApiFallback: true,
            proxy: [
                {
                    context: ['/api'],
                    target: 'http://localhost:8888',
                    changeOrigin: true,
                }
            ]
        },
        resolve: {
            extensions: ['.js', '.jsx', '.json', '.ts', '.tsx'],
            alias: {
                '@components': path.resolve(__dirname, 'src/components/'),
                '@constants': path.resolve(__dirname, 'src/constants/'),
                '@hooks': path.resolve(__dirname, 'src/hooks/'),
                '@services': path.resolve(__dirname, 'src/services/'),
                '@store': path.resolve(__dirname, 'src/store/'),
                '@styles': path.resolve(__dirname, 'src/styles/'),
                '@utils': path.resolve(__dirname, 'src/utils/'),
            }
        },
        module: {
            rules: [
                {
                    test: /\.(ts|tsx)$/,
                    loader: 'babel-loader'
                },
                {
                    test: /\.css$/i,
                    // include: path.resolve(__dirname, 'src'),
                    use: [
                        devMode ? 'style-loader' : MiniCssExtractPlugin.loader,
                        {
                            loader: "css-loader", 
                            options: {
                                sourceMap: true
                            }
                        }, 
                        {
                            loader: 'postcss-loader'
                        }
                    ],
                },
                // { 
                //     test: /\.(woff|woff2|ttf|eot)$/,  
                //     loader: "file-loader",
                //     options: {
                //         name: '[name].[contenthash].[ext]',
                //     }
                // },
                {
                    test: /\.(woff|woff2|ttf|eot)$/,
                    type: 'asset/resource',
                },
                // { 
                //     test: /\.(png|jpg|gif|svg)$/,  
                //     loader: "file-loader",
                //     options: {
                //         name: '[name].[contenthash].[ext]',
                //     }
                // },
                {
                    test: /\.(png|jpg|gif|svg)$/,
                    type: 'asset/inline'
                },
            ]
        },
        plugins: [
            // need to use ForkTsCheckerWebpackPlugin because Babel loader ignores the compilation errors for Typescript
            new ForkTsCheckerWebpackPlugin(),
            // Define plugin to inject environment variables
            new webpack.DefinePlugin({
                // Support unified API_URL with fallback to framework-specific variable
                'process.env.API_URL': JSON.stringify(process.env.API_URL || process.env.REACT_APP_API_URL),
                'process.env.REACT_APP_API_URL': JSON.stringify(process.env.API_URL || process.env.REACT_APP_API_URL),
                // Expose cloud/self-hosted flag from root .env to browser
                'process.env.LIVEREVIEW_IS_CLOUD': JSON.stringify(process.env.LIVEREVIEW_IS_CLOUD || ''),
            }),
            new MiniCssExtractPlugin({
                // Options similar to the same options in webpackOptions.output
                // both options are optional
                filename: devMode ? '[name].css' : '[name].[contenthash].css',
                chunkFilename: devMode ? '[name].css' : '[name].[contenthash].css',
            }),
            // copy static files from public folder to build directory
            new CopyPlugin({
                patterns: [
                    { 
                        from: "public/**/*", 
                        globOptions: {
                            ignore: ["**/index.html"],
                        },
                    },
                    // Copy assets folder to root of build
                    {
                        from: "assets",
                        to: "assets",
                        force: true
                    }
                ],
            }),
            new HtmlWebpackPlugin({
                template: './public/index.html',
                filename: 'index.html',
                title: metaConfig.title,
                favicon: path.resolve(__dirname, './assets/favicon.svg'),
                meta: metaConfig.meta,
                minify: {
                    html5                          : true,
                    collapseWhitespace             : true,
                    minifyCSS                      : true,
                    minifyJS                       : true,
                    minifyURLs                     : false,
                    removeComments                 : true,
                    removeEmptyAttributes          : true,
                    removeOptionalTags             : true,
                    removeRedundantAttributes      : true,
                    removeScriptTypeAttributes     : true,
                    removeStyleLinkTypeAttributese : true,
                    useShortDoctype                : true
                }
            }),
            // !devMode ? new CleanWebpackPlugin() : false,
            !devMode && process.env.ANALYZE_BUNDLE && !process.env.CI ? new BundleAnalyzerPlugin() : false,
            // Optional JavaScript obfuscation for production builds (enable with OBFUSCATE=true)
            !devMode && process.env.OBFUSCATE ? new WebpackObfuscator({
                compact: true,
                controlFlowFlattening: false,
                deadCodeInjection: false,
                debugProtection: false,
                disableConsoleOutput: true,
                identifierNamesGenerator: 'mangled',
                log: false,
                renameGlobals: false,
                rotateStringArray: true,
                selfDefending: false,
                shuffleStringArray: true,
                splitStrings: false,
                stringArray: true,
                stringArrayThreshold: 0.75,
                transformObjectKeys: false,
                unicodeEscapeSequence: false,
                // Exclude libraries from obfuscation to prevent breaking them
                exclude: /node_modules/
            }) : false
        ].filter(Boolean),
        optimization: {
            // splitChunks: {
            //     cacheGroups: {
            //         // vendor chunk
            //         vendor: {
            //             // sync + async chunks
            //             chunks: 'all',
            //             name: 'vendor',
            //             // import file path containing node_modules
            //             test: /node_modules/
            //         }
            //     }
            // },
            minimizer: [
                new TerserPlugin({
                    extractComments: false, // Don't extract comments to separate file
                    terserOptions: {
                        compress: {
                            drop_console: true,
                            drop_debugger: true,
                            pure_funcs: ['console.log', 'console.info', 'console.debug', 'console.warn'],
                            passes: 2, // Multiple compression passes
                            dead_code: true,
                            drop_debugger: true,
                            conditionals: true,
                            evaluate: true,
                            booleans: true,
                            loops: true,
                            unused: true,
                            hoist_funs: true,
                            keep_fargs: false,
                            hoist_vars: true,
                            if_return: true,
                            join_vars: true,
                            side_effects: false
                        },
                        mangle: {
                            toplevel: true,
                            eval: true,
                            keep_fnames: false,
                            properties: {
                                regex: /^_/ // Mangle properties starting with underscore
                            }
                        },
                        format: {
                            comments: false,
                            beautify: false
                        }
                    }
                }), 
                new CssMinimizerPlugin()
            ]
        },
    }

};