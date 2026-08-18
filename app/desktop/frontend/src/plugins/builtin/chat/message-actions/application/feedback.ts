import { createPublicationSlot } from "@/lib/publicationSlot";
import { RetirableTaskCohort } from "@/lib/taskQueue";
import type { MessageFeedbackRating } from "../domain/feedback";

export interface MessageFeedbackTarget {
  readonly sessionId: string;
  readonly messageId: string;
  readonly runId?: string;
}

export interface MessageFeedbackGateway {
  createMessageFeedback(input: {
    target: MessageFeedbackTarget;
    rating: MessageFeedbackRating;
  }): Promise<void>;
}

class MessageFeedbackGenerationRetiredError extends Error {
  override readonly name = "MessageFeedbackGenerationRetiredError";

  constructor() {
    super("message_feedback_generation_retired");
  }
}

type RatingListener = () => void;

class MessageFeedbackAggregate {
  readonly #target: MessageFeedbackTarget;
  readonly #gateway: MessageFeedbackGateway;
  readonly #cohort: RetirableTaskCohort;
  readonly #publish: () => void;
  #accepted: MessageFeedbackRating | undefined;
  #selected: MessageFeedbackRating | undefined;
  #latestRevision = 0;
  #latestResult: Promise<MessageFeedbackRating> | null = null;
  #tail = Promise.resolve();

  constructor(
    target: MessageFeedbackTarget,
    gateway: MessageFeedbackGateway,
    cohort: RetirableTaskCohort,
    publish: () => void,
  ) {
    this.#target = { ...target };
    this.#gateway = gateway;
    this.#cohort = cohort;
    this.#publish = publish;
  }

  rating(): MessageFeedbackRating | undefined {
    return this.#selected;
  }

  submit(rating: MessageFeedbackRating): Promise<MessageFeedbackRating> {
    this.#cohort.assertCurrent();
    if (this.#selected === rating) {
      return this.#latestResult ?? Promise.resolve(rating);
    }

    const revision = ++this.#latestRevision;
    this.#selected = rating;
    this.#publish();

    const result = this.#cohort.settle(this.#tail).then(() => this.#execute(revision, rating));
    const settlement = result.then(
      () => undefined,
      () => undefined,
    );
    this.#tail = settlement;
    this.#latestResult = result;
    void settlement.then(() => {
      if (this.#latestResult === result) this.#latestResult = null;
    });
    return result;
  }

  async #execute(revision: number, rating: MessageFeedbackRating): Promise<MessageFeedbackRating> {
    try {
      await this.#cohort.settle(
        this.#gateway.createMessageFeedback({ target: this.#target, rating }),
      );
      this.#cohort.assertCurrent();
      this.#accepted = rating;
      return rating;
    } catch (error) {
      if (!this.#cohort.retired && revision === this.#latestRevision) {
        this.#selected = this.#accepted;
        this.#publish();
      }
      throw error;
    }
  }
}

class MessageFeedbackGeneration {
  readonly #gateway: MessageFeedbackGateway;
  readonly #publish: (identity: string) => void;
  readonly #retiredError = new MessageFeedbackGenerationRetiredError();
  readonly #cohort = new RetirableTaskCohort(this.#retiredError);
  readonly #aggregates = new Map<string, MessageFeedbackAggregate>();

  constructor(gateway: MessageFeedbackGateway, publish: (identity: string) => void) {
    this.#gateway = gateway;
    this.#publish = publish;
  }

  rating(target: MessageFeedbackTarget): MessageFeedbackRating | undefined {
    return this.#aggregates.get(messageFeedbackIdentity(target))?.rating();
  }

  submit(
    target: MessageFeedbackTarget,
    rating: MessageFeedbackRating,
  ): Promise<MessageFeedbackRating> {
    this.#cohort.assertCurrent();
    const identity = messageFeedbackIdentity(target);
    let aggregate = this.#aggregates.get(identity);
    if (!aggregate) {
      aggregate = new MessageFeedbackAggregate(target, this.#gateway, this.#cohort, () => {
        this.#publish(identity);
      });
      this.#aggregates.set(identity, aggregate);
    }
    return aggregate.submit(rating);
  }

  retire(): void {
    this.#cohort.retire();
    this.#aggregates.clear();
  }
}

/** Owns feedback command ordering and optimistic material for one exact Plugin
 * Host and Runtime generation. */
export class MessageFeedbackOwner {
  static readonly #listeners = new Map<string, Set<RatingListener>>();

  readonly #gateway: MessageFeedbackGateway;
  #generation: MessageFeedbackGeneration;
  #disposed = false;

  private constructor(gateway: MessageFeedbackGateway) {
    this.#gateway = gateway;
    this.#generation = this.#newGeneration();
  }

  static install(gateway: MessageFeedbackGateway): MessageFeedbackOwner {
    const owner = new MessageFeedbackOwner(gateway);
    messageFeedbackPublication.publish(owner, (predecessor) => predecessor.dispose());
    MessageFeedbackOwner.#publishAll();
    return owner;
  }

  static current(): MessageFeedbackOwner {
    const owner = messageFeedbackPublication.current();
    if (!owner || owner.#disposed) throw new Error("Message feedback owner is not installed");
    return owner;
  }

  static rating(target: MessageFeedbackTarget): MessageFeedbackRating | undefined {
    const owner = messageFeedbackPublication.current();
    return owner && !owner.#disposed ? owner.#generation.rating(target) : undefined;
  }

  static subscribe(target: MessageFeedbackTarget, listener: RatingListener): () => void {
    const identity = messageFeedbackIdentity(target);
    let listeners = MessageFeedbackOwner.#listeners.get(identity);
    if (!listeners) {
      listeners = new Set();
      MessageFeedbackOwner.#listeners.set(identity, listeners);
    }
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
      if (listeners.size === 0) MessageFeedbackOwner.#listeners.delete(identity);
    };
  }

  submit(
    target: MessageFeedbackTarget,
    rating: MessageFeedbackRating,
  ): Promise<MessageFeedbackRating> {
    return this.#generation.submit(target, rating);
  }

  replaceRuntimeGeneration(): void {
    if (this.#disposed || !messageFeedbackPublication.owns(this)) return;
    const predecessor = this.#generation;
    this.#generation = this.#newGeneration();
    predecessor.retire();
    MessageFeedbackOwner.#publishAll();
  }

  dispose(): void {
    if (this.#disposed) return;
    this.#disposed = true;
    this.#generation.retire();
    if (messageFeedbackPublication.withdraw(this)) {
      MessageFeedbackOwner.#publishAll();
    }
  }

  static #publishAll(): void {
    for (const listeners of MessageFeedbackOwner.#listeners.values()) {
      for (const listener of listeners) listener();
    }
  }

  static #publish(identity: string): void {
    for (const listener of MessageFeedbackOwner.#listeners.get(identity) ?? []) listener();
  }

  #newGeneration(): MessageFeedbackGeneration {
    return new MessageFeedbackGeneration(this.#gateway, (identity) =>
      MessageFeedbackOwner.#publish(identity),
    );
  }
}

const messageFeedbackPublication = createPublicationSlot<MessageFeedbackOwner>();

export function messageFeedbackRating(
  target: MessageFeedbackTarget,
): MessageFeedbackRating | undefined {
  return MessageFeedbackOwner.rating(target);
}

export function subscribeMessageFeedback(
  target: MessageFeedbackTarget,
  listener: RatingListener,
): () => void {
  return MessageFeedbackOwner.subscribe(target, listener);
}

export function submitMessageFeedback(
  target: MessageFeedbackTarget,
  rating: MessageFeedbackRating,
): Promise<MessageFeedbackRating> {
  return MessageFeedbackOwner.current().submit(target, rating);
}

export function messageFeedbackWasRetired(error: unknown): boolean {
  return error instanceof MessageFeedbackGenerationRetiredError;
}

function messageFeedbackIdentity(target: MessageFeedbackTarget): string {
  return JSON.stringify([target.sessionId, target.messageId]);
}
