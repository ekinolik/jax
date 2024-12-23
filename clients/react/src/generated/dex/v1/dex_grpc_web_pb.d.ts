import * as grpcWeb from 'grpc-web';

import * as dex_v1_dex_pb from '../../dex/v1/dex_pb'; // proto import: "dex/v1/dex.proto"


export class DexServiceClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  getDex(
    request: dex_v1_dex_pb.GetDexRequest,
    metadata: grpcWeb.Metadata | undefined,
    callback: (err: grpcWeb.RpcError,
               response: dex_v1_dex_pb.GetDexResponse) => void
  ): grpcWeb.ClientReadableStream<dex_v1_dex_pb.GetDexResponse>;

}

export class DexServicePromiseClient {
  constructor (hostname: string,
               credentials?: null | { [index: string]: string; },
               options?: null | { [index: string]: any; });

  getDex(
    request: dex_v1_dex_pb.GetDexRequest,
    metadata?: grpcWeb.Metadata
  ): Promise<dex_v1_dex_pb.GetDexResponse>;

}

