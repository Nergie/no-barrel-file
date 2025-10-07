import { BasicClass } from "./classes";

export { BasicClass as RenamedBasicClass };

import { BasicType } from "./types";

type ReExportedBasicType = BasicType;
export type { ReExportedBasicType };

const ReExportedBasicConstToExport = 42;
export { ReExportedBasicConstToExport };

export * as DefaultExported from "./default-export";