const { defineConfig } = require('@vue/cli-service')
const path = require('path')

module.exports = defineConfig({
  transpileDependencies: [
    'vuetify'
  ],
  configureWebpack: {
    resolve: {
      alias: {
        '@': path.resolve(__dirname, 'src')
      }
    }
  },
  devServer: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  },
  lintOnSave: false,
  css: {
    loaderOptions: {
      sass: {
        sassOptions: {
          quietDeps: true,
          silenceDeprecations: ['legacy-js-api']
        }
      }
    }
  }
})
