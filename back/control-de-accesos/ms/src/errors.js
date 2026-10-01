export class DomainError extends Error {
  constructor(message, status) {
    super(message);
    this.status = status;
  }
}

export const errors = {
  notFound: () => new DomainError("visitor pass not found", 404),
  invalidToken: () => new DomainError("invalid or unknown pass token", 404),
  unitNotInComplex: () => new DomainError("unit does not belong to this complex", 403),
  unitAccessDenied: () => new DomainError("you do not have access to this unit", 403),
  passNotActive: () => new DomainError("pass is no longer active", 409),
  passExpired: () => new DomainError("pass has expired", 409),
  passNotYetValid: () => new DomainError("pass is not valid yet", 409),
  passNotCheckedIn: () => new DomainError("pass is not checked in", 409),
  passNotExtendable: () => new DomainError("only an active, unexpired pass can be extended", 409),
  invalidExtension: () =>
    new DomainError("new expiry must be later and within the maximum validity window", 400),
  visitorNameRequired: () => new DomainError("visitor name is required", 400),
  invalidWindow: () => new DomainError("invalid validity window", 400),
  invalidPlate: () => new DomainError("plate is required", 400),
  invalidDirection: () => new DomainError("direction must be in or out", 400),
  preauthNotFound: () => new DomainError("visitor pre-authorization not found", 404),
  unauthorized: () => new DomainError("invalid user", 401),
  forbiddenRole: () => new DomainError("this action is not allowed for your role", 403),
};

export function sendError(res, error) {
  if (error instanceof DomainError) {
    res.status(error.status).json({ error: error.message });
    return;
  }
  console.error(error);
  res.status(500).json({ error: "An unexpected error occurred" });
}
