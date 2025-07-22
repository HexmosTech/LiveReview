# AI Providers Module

This module manages the AI providers and connectors for the LiveReview application.

## Structure

The module is organized as follows:

```
AIProviders/
├── components/              # UI components
│   ├── ConnectorCard.tsx    # Individual connector card
│   ├── ConnectorForm.tsx    # Form for adding/editing connectors
│   ├── ConnectorsList.tsx   # List of all connectors
│   ├── ProviderDetail.tsx   # Provider details panel
│   ├── ProvidersList.tsx    # Left sidebar with provider list
│   ├── UsageTips.tsx        # Usage tips panel
│   └── index.ts             # Exports all components
├── context/                 # React Context API
│   └── AIProvidersContext.tsx # Context provider for shared state
├── hooks/                   # Custom React hooks
│   ├── useConnectors.ts     # Hook for connector data and operations
│   ├── useFormState.ts      # Hook for form state management
│   ├── useProviderSelection.ts # Hook for provider selection and URL state
│   └── index.ts             # Exports all hooks
├── types/                   # TypeScript type definitions
│   └── index.ts             # Contains all type definitions
├── utils/                   # Utility functions
│   ├── apiUtils.ts          # API-related utility functions
│   └── nameUtils.ts         # Name generation and provider utilities
└── AIProviders.tsx          # Main component file
```

## How to Use

The AIProviders component is the main entry point for this module. It handles the overall layout and orchestrates the interaction between the different components.

## Refactoring Details

This module was refactored from a large monolithic component to a more modular structure with:

1. **Separated Components**: Each UI element is now its own component
2. **Custom Hooks**: State management is handled by custom hooks
3. **Type Definitions**: Clear type definitions for improved type safety
4. **Utility Functions**: Common functions moved to utility files
5. **Context API**: Optional React Context for more efficient state sharing between components

## API Integration

The module integrates with the backend API through the utility functions in `apiUtils.ts`. These functions handle:

- Fetching connectors
- Validating API keys
- Creating new connectors
- Updating existing connectors

## URL Routing

The module uses React Router for handling URL routing. The URLs follow this pattern:

- `/ai/all` - View all connectors
- `/ai/{providerId}` - View connectors for a specific provider
- `/ai/{providerId}/add` - Add a new connector for a provider
- `/ai/{providerId}/edit/{connectorId}` - Edit a specific connector
