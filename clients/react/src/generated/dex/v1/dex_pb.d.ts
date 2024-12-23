import * as jspb from 'google-protobuf'



export class GetDexRequest extends jspb.Message {
  getUnderlyingAsset(): string;
  setUnderlyingAsset(value: string): GetDexRequest;

  getStartStrikePrice(): number;
  setStartStrikePrice(value: number): GetDexRequest;
  hasStartStrikePrice(): boolean;
  clearStartStrikePrice(): GetDexRequest;

  getEndStrikePrice(): number;
  setEndStrikePrice(value: number): GetDexRequest;
  hasEndStrikePrice(): boolean;
  clearEndStrikePrice(): GetDexRequest;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): GetDexRequest.AsObject;
  static toObject(includeInstance: boolean, msg: GetDexRequest): GetDexRequest.AsObject;
  static serializeBinaryToWriter(message: GetDexRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): GetDexRequest;
  static deserializeBinaryFromReader(message: GetDexRequest, reader: jspb.BinaryReader): GetDexRequest;
}

export namespace GetDexRequest {
  export type AsObject = {
    underlyingAsset: string,
    startStrikePrice?: number,
    endStrikePrice?: number,
  }

  export enum StartStrikePriceCase { 
    _START_STRIKE_PRICE_NOT_SET = 0,
    START_STRIKE_PRICE = 2,
  }

  export enum EndStrikePriceCase { 
    _END_STRIKE_PRICE_NOT_SET = 0,
    END_STRIKE_PRICE = 3,
  }
}

export class GetDexResponse extends jspb.Message {
  getSpotPrice(): number;
  setSpotPrice(value: number): GetDexResponse;

  getStrikePricesMap(): jspb.Map<string, ExpirationDateMap>;
  clearStrikePricesMap(): GetDexResponse;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): GetDexResponse.AsObject;
  static toObject(includeInstance: boolean, msg: GetDexResponse): GetDexResponse.AsObject;
  static serializeBinaryToWriter(message: GetDexResponse, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): GetDexResponse;
  static deserializeBinaryFromReader(message: GetDexResponse, reader: jspb.BinaryReader): GetDexResponse;
}

export namespace GetDexResponse {
  export type AsObject = {
    spotPrice: number,
    strikePricesMap: Array<[string, ExpirationDateMap.AsObject]>,
  }
}

export class ExpirationDateMap extends jspb.Message {
  getExpirationDatesMap(): jspb.Map<string, OptionTypeMap>;
  clearExpirationDatesMap(): ExpirationDateMap;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): ExpirationDateMap.AsObject;
  static toObject(includeInstance: boolean, msg: ExpirationDateMap): ExpirationDateMap.AsObject;
  static serializeBinaryToWriter(message: ExpirationDateMap, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): ExpirationDateMap;
  static deserializeBinaryFromReader(message: ExpirationDateMap, reader: jspb.BinaryReader): ExpirationDateMap;
}

export namespace ExpirationDateMap {
  export type AsObject = {
    expirationDatesMap: Array<[string, OptionTypeMap.AsObject]>,
  }
}

export class OptionTypeMap extends jspb.Message {
  getOptionTypesMap(): jspb.Map<string, DexValue>;
  clearOptionTypesMap(): OptionTypeMap;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): OptionTypeMap.AsObject;
  static toObject(includeInstance: boolean, msg: OptionTypeMap): OptionTypeMap.AsObject;
  static serializeBinaryToWriter(message: OptionTypeMap, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): OptionTypeMap;
  static deserializeBinaryFromReader(message: OptionTypeMap, reader: jspb.BinaryReader): OptionTypeMap;
}

export namespace OptionTypeMap {
  export type AsObject = {
    optionTypesMap: Array<[string, DexValue.AsObject]>,
  }
}

export class DexValue extends jspb.Message {
  getValue(): number;
  setValue(value: number): DexValue;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): DexValue.AsObject;
  static toObject(includeInstance: boolean, msg: DexValue): DexValue.AsObject;
  static serializeBinaryToWriter(message: DexValue, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): DexValue;
  static deserializeBinaryFromReader(message: DexValue, reader: jspb.BinaryReader): DexValue;
}

export namespace DexValue {
  export type AsObject = {
    value: number,
  }
}

