package spec_test

import (
	"math/rand"

	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo"
	. "github.com/yyleeshine/mpquic/repository/onsi/ginkgo/internal/spec"
	. "github.com/yyleeshine/mpquic/repository/onsi/gomega"

	"github.com/yyleeshine/mpquic/repository/onsi/ginkgo/internal/codelocation"
	"github.com/yyleeshine/mpquic/repository/onsi/ginkgo/internal/containernode"
	"github.com/yyleeshine/mpquic/repository/onsi/ginkgo/internal/leafnodes"
	"github.com/yyleeshine/mpquic/repository/onsi/ginkgo/types"
)

var _ = Describe("Specs", func() {
	var specs *Specs

	newSpec := func(text string, flag types.FlagType) *Spec {
		subject := leafnodes.NewItNode(text, func() {}, flag, codelocation.New(0), 0, nil, 0)
		return New(subject, []*containernode.ContainerNode{}, false)
	}

	newMeasureSpec := func(text string, flag types.FlagType) *Spec {
		subject := leafnodes.NewMeasureNode(text, func(Benchmarker) {}, flag, codelocation.New(0), 0, nil, 0)
		return New(subject, []*containernode.ContainerNode{}, false)
	}

	newSpecs := func(args ...interface{}) *Specs {
		specs := []*Spec{}
		for index := 0; index < len(args)-1; index += 2 {
			specs = append(specs, newSpec(args[index].(string), args[index+1].(types.FlagType)))
		}
		return NewSpecs(specs)
	}

	specTexts := func(specs *Specs) []string {
		texts := []string{}
		for _, spec := range specs.Specs() {
			texts = append(texts, spec.ConcatenatedString())
		}
		return texts
	}

	willRunTexts := func(specs *Specs) []string {
		texts := []string{}
		for _, spec := range specs.Specs() {
			if !(spec.Skipped() || spec.Pending()) {
				texts = append(texts, spec.ConcatenatedString())
			}
		}
		return texts
	}

	skippedTexts := func(specs *Specs) []string {
		texts := []string{}
		for _, spec := range specs.Specs() {
			if spec.Skipped() {
				texts = append(texts, spec.ConcatenatedString())
			}
		}
		return texts
	}

	pendingTexts := func(specs *Specs) []string {
		texts := []string{}
		for _, spec := range specs.Specs() {
			if spec.Pending() {
				texts = append(texts, spec.ConcatenatedString())
			}
		}
		return texts
	}

	Describe("Shuffling specs", func() {
		It("should shuffle the specs using the passed in randomizer", func() {
			specs17 := newSpecs("C", noneFlag, "A", noneFlag, "B", noneFlag)
			specs17.Shuffle(rand.New(rand.NewSource(17)))
			texts17 := specTexts(specs17)

			specs17Again := newSpecs("C", noneFlag, "A", noneFlag, "B", noneFlag)
			specs17Again.Shuffle(rand.New(rand.NewSource(17)))
			texts17Again := specTexts(specs17Again)

			specs15 := newSpecs("C", noneFlag, "A", noneFlag, "B", noneFlag)
			specs15.Shuffle(rand.New(rand.NewSource(15)))
			texts15 := specTexts(specs15)

			specsUnshuffled := newSpecs("C", noneFlag, "A", noneFlag, "B", noneFlag)
			textsUnshuffled := specTexts(specsUnshuffled)

			??(textsUnshuffled).Should(Equal([]string{"C", "A", "B"}))

			??(texts17).Should(Equal(texts17Again))
			??(texts17).ShouldNot(Equal(texts15))
			??(texts17).ShouldNot(Equal(textsUnshuffled))
			??(texts15).ShouldNot(Equal(textsUnshuffled))

			??(texts17).Should(HaveLen(3))
			??(texts17).Should(ContainElement("A"))
			??(texts17).Should(ContainElement("B"))
			??(texts17).Should(ContainElement("C"))

			??(texts15).Should(HaveLen(3))
			??(texts15).Should(ContainElement("A"))
			??(texts15).Should(ContainElement("B"))
			??(texts15).Should(ContainElement("C"))
		})
	})

	Describe("with no programmatic focus", func() {
		BeforeEach(func() {
			specs = newSpecs("A1", noneFlag, "A2", noneFlag, "B1", noneFlag, "B2", pendingFlag)
			specs.ApplyFocus("", "", "")
		})

		It("should not report as having programmatic specs", func() {
			??(specs.HasProgrammaticFocus()).Should(BeFalse())
		})
	})

	Describe("Applying focus/skip", func() {
		var description, focusString, skipString string

		BeforeEach(func() {
			description, focusString, skipString = "", "", ""
		})

		JustBeforeEach(func() {
			specs = newSpecs("A1", focusedFlag, "A2", noneFlag, "B1", focusedFlag, "B2", pendingFlag)
			specs.ApplyFocus(description, focusString, skipString)
		})

		Context("with neither a focus string nor a skip string", func() {
			It("should apply the programmatic focus", func() {
				??(willRunTexts(specs)).Should(Equal([]string{"A1", "B1"}))
				??(skippedTexts(specs)).Should(Equal([]string{"A2", "B2"}))
				??(pendingTexts(specs)).Should(BeEmpty())
			})

			It("should report as having programmatic specs", func() {
				??(specs.HasProgrammaticFocus()).Should(BeTrue())
			})
		})

		Context("with a focus regexp", func() {
			BeforeEach(func() {
				focusString = "A"
			})

			It("should override the programmatic focus", func() {
				??(willRunTexts(specs)).Should(Equal([]string{"A1", "A2"}))
				??(skippedTexts(specs)).Should(Equal([]string{"B1", "B2"}))
				??(pendingTexts(specs)).Should(BeEmpty())
			})

			It("should not report as having programmatic specs", func() {
				??(specs.HasProgrammaticFocus()).Should(BeFalse())
			})
		})

		Context("with a focus regexp", func() {
			BeforeEach(func() {
				focusString = "B"
			})

			It("should not override any pendings", func() {
				??(willRunTexts(specs)).Should(Equal([]string{"B1"}))
				??(skippedTexts(specs)).Should(Equal([]string{"A1", "A2"}))
				??(pendingTexts(specs)).Should(Equal([]string{"B2"}))
			})
		})

		Context("with a description", func() {
			BeforeEach(func() {
				description = "C"
				focusString = "C"
			})

			It("should include the description in the focus determination", func() {
				??(willRunTexts(specs)).Should(Equal([]string{"A1", "A2", "B1"}))
				??(skippedTexts(specs)).Should(BeEmpty())
				??(pendingTexts(specs)).Should(Equal([]string{"B2"}))
			})
		})

		Context("with a description", func() {
			BeforeEach(func() {
				description = "C"
				skipString = "C"
			})

			It("should include the description in the focus determination", func() {
				??(willRunTexts(specs)).Should(BeEmpty())
				??(skippedTexts(specs)).Should(Equal([]string{"A1", "A2", "B1", "B2"}))
				??(pendingTexts(specs)).Should(BeEmpty())
			})
		})

		Context("with a skip regexp", func() {
			BeforeEach(func() {
				skipString = "A"
			})

			It("should override the programmatic focus", func() {
				??(willRunTexts(specs)).Should(Equal([]string{"B1"}))
				??(skippedTexts(specs)).Should(Equal([]string{"A1", "A2"}))
				??(pendingTexts(specs)).Should(Equal([]string{"B2"}))
			})

			It("should not report as having programmatic specs", func() {
				??(specs.HasProgrammaticFocus()).Should(BeFalse())
			})
		})

		Context("with both a focus and a skip regexp", func() {
			BeforeEach(func() {
				focusString = "1"
				skipString = "B"
			})

			It("should AND the two", func() {
				??(willRunTexts(specs)).Should(Equal([]string{"A1"}))
				??(skippedTexts(specs)).Should(Equal([]string{"A2", "B1", "B2"}))
				??(pendingTexts(specs)).Should(BeEmpty())
			})

			It("should not report as having programmatic specs", func() {
				??(specs.HasProgrammaticFocus()).Should(BeFalse())
			})
		})
	})

	Describe("With a focused spec within a pending context and a pending spec within a focused context", func() {
		BeforeEach(func() {
			pendingInFocused := New(
				leafnodes.NewItNode("PendingInFocused", func() {}, pendingFlag, codelocation.New(0), 0, nil, 0),
				[]*containernode.ContainerNode{
					containernode.New("", focusedFlag, codelocation.New(0)),
				}, false)

			focusedInPending := New(
				leafnodes.NewItNode("FocusedInPending", func() {}, focusedFlag, codelocation.New(0), 0, nil, 0),
				[]*containernode.ContainerNode{
					containernode.New("", pendingFlag, codelocation.New(0)),
				}, false)

			specs = NewSpecs([]*Spec{
				newSpec("A", noneFlag),
				newSpec("B", noneFlag),
				pendingInFocused,
				focusedInPending,
			})
			specs.ApplyFocus("", "", "")
		})

		It("should not have a programmatic focus and should run all tests", func() {
			??(willRunTexts(specs)).Should(Equal([]string{"A", "B"}))
			??(skippedTexts(specs)).Should(BeEmpty())
			??(pendingTexts(specs)).Should(ConsistOf(ContainSubstring("PendingInFocused"), ContainSubstring("FocusedInPending")))
		})
	})

	Describe("skipping measurements", func() {
		BeforeEach(func() {
			specs = NewSpecs([]*Spec{
				newSpec("A", noneFlag),
				newSpec("B", noneFlag),
				newSpec("C", pendingFlag),
				newMeasureSpec("measurementA", noneFlag),
				newMeasureSpec("measurementB", pendingFlag),
			})
		})

		It("should skip measurements", func() {
			??(willRunTexts(specs)).Should(Equal([]string{"A", "B", "measurementA"}))
			??(skippedTexts(specs)).Should(BeEmpty())
			??(pendingTexts(specs)).Should(Equal([]string{"C", "measurementB"}))

			specs.SkipMeasurements()

			??(willRunTexts(specs)).Should(Equal([]string{"A", "B"}))
			??(skippedTexts(specs)).Should(Equal([]string{"measurementA", "measurementB"}))
			??(pendingTexts(specs)).Should(Equal([]string{"C"}))
		})
	})
})
