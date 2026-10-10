import { YuvaPageFeedbackElement, type PageFeedbackDetail, type PageRatingDetail } from "./docs/feedback";
import { YuvaPageQuestionsElement, type PageQuestionDetail } from "./docs/questions";

export function defineDocs(): void {
  if (typeof customElements === "undefined") return;
  if (!customElements.get("yuva-page-feedback")) customElements.define("yuva-page-feedback", YuvaPageFeedbackElement);
  if (!customElements.get("yuva-page-questions")) customElements.define("yuva-page-questions", YuvaPageQuestionsElement);
}

defineDocs();

export { YuvaPageFeedbackElement, YuvaPageQuestionsElement, type PageFeedbackDetail, type PageQuestionDetail, type PageRatingDetail };
export type { IdentityTokenSource } from "./client/client";
export type { ClientPageAnswer, PageRating } from "./client/api";
