import { grpc } from '@improbable-eng/grpc-web';
import { DexService } from './generated/dex_pb_service';
import { GetDexRequest, GetDexResponse } from './generated/dex_pb';

export interface DexOptions {
  host: string;
}

export interface GetDexParams {
  underlyingAsset: string;
  startStrikePrice?: number;
  endStrikePrice?: number;
}

export class JaxClient {
  private host: string;

  constructor(options: DexOptions) {
    this.host = options.host;
  }

  getDex(params: GetDexParams): Promise<GetDexResponse> {
    return new Promise((resolve, reject) => {
      const request = new GetDexRequest();
      request.setUnderlyingAsset(params.underlyingAsset);
      
      if (params.startStrikePrice !== undefined) {
        request.setStartStrikePrice(params.startStrikePrice);
      }
      if (params.endStrikePrice !== undefined) {
        request.setEndStrikePrice(params.endStrikePrice);
      }

      grpc.unary(DexService.GetDex, {
        request,
        host: this.host,
        onEnd: (response) => {
          const { status, statusMessage, message } = response;
          if (status === grpc.Code.OK && message) {
            resolve(message as GetDexResponse);
          } else {
            reject(new Error(statusMessage));
          }
        },
      });
    });
  }
}

// React hook for using the JAX client
export function useJaxClient(options: DexOptions) {
  const client = new JaxClient(options);

  return {
    getDex: (params: GetDexParams) => client.getDex(params),
  };
} 