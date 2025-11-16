import { BasicClass } from "./classes";

export { BasicClass as RenamedBasicClass };

import { BasicType } from "./types";

type ReExportedBasicType = BasicType;
export type { ReExportedBasicType };

const ReExportedBasicConstToExport = 42;
export { ReExportedBasicConstToExport };

export * as DefaultObject from "./default-object";
export * as DefaultFunction from "./default-function";

export { default as RenamedDefaultObject } from './default-object'
export { default as RenamedDefaultFunction } from './default-function'
