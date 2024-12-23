/**
 * @fileoverview gRPC-Web generated client stub for dex.v1
 * @enhanceable
 * @public
 */

// Code generated by protoc-gen-grpc-web. DO NOT EDIT.
// versions:
// 	protoc-gen-grpc-web v1.5.0
// 	protoc              v5.29.2
// source: dex/v1/dex.proto


/* eslint-disable */
// @ts-nocheck



const grpc = {};
grpc.web = require('grpc-web');

const proto = {};
proto.dex = {};
proto.dex.v1 = require('./dex_pb.js');

/**
 * @param {string} hostname
 * @param {?Object} credentials
 * @param {?grpc.web.ClientOptions} options
 * @constructor
 * @struct
 * @final
 */
proto.dex.v1.DexServiceClient =
    function(hostname, credentials, options) {
  if (!options) options = {};
  options.format = 'text';

  /**
   * @private @const {!grpc.web.GrpcWebClientBase} The client
   */
  this.client_ = new grpc.web.GrpcWebClientBase(options);

  /**
   * @private @const {string} The hostname
   */
  this.hostname_ = hostname.replace(/\/+$/, '');

};


/**
 * @param {string} hostname
 * @param {?Object} credentials
 * @param {?grpc.web.ClientOptions} options
 * @constructor
 * @struct
 * @final
 */
proto.dex.v1.DexServicePromiseClient =
    function(hostname, credentials, options) {
  if (!options) options = {};
  options.format = 'text';

  /**
   * @private @const {!grpc.web.GrpcWebClientBase} The client
   */
  this.client_ = new grpc.web.GrpcWebClientBase(options);

  /**
   * @private @const {string} The hostname
   */
  this.hostname_ = hostname.replace(/\/+$/, '');

};


/**
 * @const
 * @type {!grpc.web.MethodDescriptor<
 *   !proto.dex.v1.GetDexRequest,
 *   !proto.dex.v1.GetDexResponse>}
 */
const methodDescriptor_DexService_GetDex = new grpc.web.MethodDescriptor(
  '/dex.v1.DexService/GetDex',
  grpc.web.MethodType.UNARY,
  proto.dex.v1.GetDexRequest,
  proto.dex.v1.GetDexResponse,
  /**
   * @param {!proto.dex.v1.GetDexRequest} request
   * @return {!Uint8Array}
   */
  function(request) {
    return request.serializeBinary();
  },
  proto.dex.v1.GetDexResponse.deserializeBinary
);


/**
 * @param {!proto.dex.v1.GetDexRequest} request The
 *     request proto
 * @param {?Object<string, string>} metadata User defined
 *     call metadata
 * @param {function(?grpc.web.RpcError, ?proto.dex.v1.GetDexResponse)}
 *     callback The callback function(error, response)
 * @return {!grpc.web.ClientReadableStream<!proto.dex.v1.GetDexResponse>|undefined}
 *     The XHR Node Readable Stream
 */
proto.dex.v1.DexServiceClient.prototype.getDex =
    function(request, metadata, callback) {
  return this.client_.rpcCall(this.hostname_ +
      '/dex.v1.DexService/GetDex',
      request,
      metadata || {},
      methodDescriptor_DexService_GetDex,
      callback);
};


/**
 * @param {!proto.dex.v1.GetDexRequest} request The
 *     request proto
 * @param {?Object<string, string>=} metadata User defined
 *     call metadata
 * @return {!Promise<!proto.dex.v1.GetDexResponse>}
 *     Promise that resolves to the response
 */
proto.dex.v1.DexServicePromiseClient.prototype.getDex =
    function(request, metadata) {
  return this.client_.unaryCall(this.hostname_ +
      '/dex.v1.DexService/GetDex',
      request,
      metadata || {},
      methodDescriptor_DexService_GetDex);
};


module.exports = proto.dex.v1;

