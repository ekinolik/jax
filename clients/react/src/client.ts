import { grpc } from '@improbable-eng/grpc-web';
import { DexServiceClient } from './generated/dex/v1/DexServiceClientPb';
import { GetDexRequest, GetDexResponse } from './generated/dex/v1/dex_pb';

export interface DexOptions {
  host: string;
}

export interface GetDexParams {
  underlyingAsset: string;
  startStrikePrice?: number;
  endStrikePrice?: number;
}

export class JaxClient {
  private client: DexServiceClient;

  constructor(options: DexOptions) {
    this.client = new DexServiceClient(options.host);
  }

  getDex(params: GetDexParams): Promise<GetDexResponse> {
    const request = new GetDexRequest();
    request.setUnderlyingAsset(params.underlyingAsset);
    
    if (params.startStrikePrice !== undefined) {
      request.setStartStrikePrice(params.startStrikePrice);
    }
    if (params.endStrikePrice !== undefined) {
      request.setEndStrikePrice(params.endStrikePrice);
    }

    return this.client.getDex(request);
  }
}

// React hook for using the JAX client
export function useJaxClient(options: DexOptions) {
  const client = new JaxClient(options);

  return {
    getDex: (params: GetDexParams) => client.getDex(params),
  };
} 