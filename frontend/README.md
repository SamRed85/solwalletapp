# Sol Frontend

A modern Vue.js frontend with Vuetify 2 for the Sol Backend API.

## Features

- 🎨 Beautiful Material Design UI with Vuetify 2
- 📱 Responsive design for all devices
- 🔄 Real-time backend health monitoring
- 👥 User management with CRUD operations
- 🚀 Modern Vue.js 2 with Vue Router
- 📊 Data tables with sorting and actions

## Prerequisites

- Node.js 14 or higher
- npm or yarn

## Setup

1. **Install dependencies:**
   ```bash
   cd frontend
   npm install
   ```

2. **Start the development server:**
   ```bash
   npm run serve
   ```

The frontend will be available at `http://localhost:3000`

## Available Scripts

- `npm run serve` - Start development server
- `npm run build` - Build for production
- `npm run lint` - Run ESLint

## Project Structure

```
frontend/
├── public/
│   └── index.html          # Main HTML template
├── src/
│   ├── components/         # Vue components
│   ├── views/             # Page components
│   │   ├── Home.vue       # Home page
│   │   └── Users.vue      # User management page
│   ├── services/          # API services
│   │   └── api.js         # API client
│   ├── router/            # Vue Router configuration
│   │   └── index.js
│   ├── plugins/           # Vue plugins
│   │   └── vuetify.js     # Vuetify configuration
│   ├── App.vue            # Main app component
│   └── main.js            # App entry point
├── package.json           # Dependencies
├── vue.config.js          # Vue CLI configuration
└── jsconfig.json          # Path aliases
```

## API Integration

The frontend communicates with the backend API through the `api.js` service:

- **Health Service**: Check backend status
- **User Service**: CRUD operations for users

## Configuration

- **API Base URL**: Configured in `src/services/api.js`
- **Development**: `http://localhost:8080`
- **Production**: Update the `API_BASE_URL` constant

## Styling

- **Vuetify 2**: Material Design components
- **Material Design Icons**: Icon library
- **Roboto Font**: Google Fonts integration
- **Custom Themes**: Configurable in `src/plugins/vuetify.js`

## Browser Support

- Chrome (latest)
- Firefox (latest)
- Safari (latest)
- Edge (latest)
