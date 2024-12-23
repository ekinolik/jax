# JAX React Client

A React client library for the JAX (Options Delta Exposure) service.

## Installation

```bash
npm install @ekinolik/jax-client
# or
yarn add @ekinolik/jax-client
```

## Usage

### Using the React Hook

```tsx
import { useJaxClient } from '@ekinolik/jax-client';

function App() {
  const jaxClient = useJaxClient({
    host: 'http://localhost:50051',
  });

  const handleFetchData = async () => {
    try {
      const response = await jaxClient.getDex({
        underlyingAsset: 'AAPL',
        startStrikePrice: 170,
        endStrikePrice: 180,
      });
      console.log('DEX data:', response);
    } catch (error) {
      console.error('Error:', error);
    }
  };

  return (
    <button onClick={handleFetchData}>
      Fetch DEX Data
    </button>
  );
}
```

### Using the DexViewer Component

```tsx
import { DexViewer } from '@ekinolik/jax-client/components';

function App() {
  return (
    <DexViewer
      host="http://localhost:50051"
      underlyingAsset="AAPL"
      startStrike={170}
      endStrike={180}
    />
  );
}
```

## Response Structure

The response includes:
- `spotPrice`: Current price of the underlying asset
- `strikePrices`: Map of strike prices to expiration dates and option types, with DEX values

Example response:
```json
{
  "spotPrice": 123.45,
  "strikePrices": {
    "170": {
      "expirationDates": {
        "2024-03-15": {
          "optionTypes": {
            "call": {
              "value": 12345.67
            },
            "put": {
              "value": -1234.56
            }
          }
        }
      }
    }
  }
}
```

## Requirements

- React 16.8.0 or later
- A running JAX service
- gRPC-Web proxy (e.g., Envoy) for browser compatibility

## Development

1. Install dependencies:
```bash
npm install
```

2. Build the package:
```bash
npm run build
```

## Notes

- Make sure you have a gRPC-Web proxy (like Envoy) running in front of your JAX service for browser compatibility.
- The service host URL should point to your gRPC-Web proxy, not directly to the JAX service. 